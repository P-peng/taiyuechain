// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package p2p

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/hex"
	"github.com/taiyuechain/taiyuechain/crypto"
	"reflect"

	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/taiyuechain/taiyuechain/log"
	"hash"
	"io"
	"io/ioutil"
	"net"
	"sync"
	"time"

	"github.com/golang/snappy"
	"github.com/taiyuechain/taiyuechain/common/bitutil"
	"github.com/taiyuechain/taiyuechain/rlp"

	"golang.org/x/crypto/sha3"
)

const (
	maxUint24 = ^uint32(0) >> 8

	sskLen            = 16 // ecies.MaxSharedKeyLength(pubKey) / 2
	sigLen            = 98 // elliptic S256
	pubLen            = 64 // 512 bit pubkey in uncompressed representation without format byte
	shaLen            = 32 // hash length (for nonce etc)
	authRLPDecLen     = -25
	authRespRLPIncLen = 5
	certSize          = 2

	authMsgLen  = sigLen + shaLen + pubLen + shaLen + 1 + authRLPDecLen + certSize
	authRespLen = pubLen + shaLen + 1 + authRespRLPIncLen + certSize

	eciesOverhead = 65 /* pubkey */ + 16 /* IV */ + 32 /* MAC */

	encAuthMsgLen  = authMsgLen + eciesOverhead  // size of encrypted pre-EIP-8 initiator handshake
	encAuthRespLen = authRespLen + eciesOverhead // size of encrypted pre-EIP-8 handshake reply

	// total timeout for encryption handshake and protocol
	// handshake in both directions.
	handshakeTimeout = 5 * time.Second

	// This is the timeout for sending the disconnect reason.
	// This is shorter than the usual timeout because we don't want
	// to wait if the connection is known to be bad anyway.
	discWriteTimeout = 1 * time.Second

	TaiRLPXVersion = 15
)

// errPlainMessageTooLarge is returned if a decompressed message length exceeds
// the allowed 24 bits (i.e. length >= 16MB).
var errPlainMessageTooLarge = errors.New("message length >= 16MB")

// rlpx is the transport protocol used by actual (non-test) connections.
// It wraps the frame encoder with locks and read/write deadlines.
type rlpx struct {
	fd net.Conn
	cm *certManager

	rmu, wmu sync.Mutex
	rw       *rlpxFrameRW
}

func newRLPX(fd net.Conn, cm *certManager) transport {
	fd.SetDeadline(time.Now().Add(handshakeTimeout))
	return &rlpx{fd: fd, cm: cm}
}

func (t *rlpx) ReadMsg() (Msg, error) {
	t.rmu.Lock()
	defer t.rmu.Unlock()
	t.fd.SetReadDeadline(time.Now().Add(frameReadTimeout))
	return t.rw.ReadMsg()
}

func (t *rlpx) WriteMsg(msg Msg) error {
	t.wmu.Lock()
	defer t.wmu.Unlock()
	t.fd.SetWriteDeadline(time.Now().Add(frameWriteTimeout))
	return t.rw.WriteMsg(msg)
}

func (t *rlpx) close(err error) {
	t.wmu.Lock()
	defer t.wmu.Unlock()
	// Tell the remote end why we're disconnecting if possible.
	if t.rw != nil {
		if r, ok := err.(DiscReason); ok && r != DiscNetworkError {
			// rlpx tries to send DiscReason to disconnected peer
			// if the connection is net.Pipe (in-memory simulation)
			// it hangs forever, since net.Pipe does not implement
			// a write deadline. Because of this only try to send
			// the disconnect reason message if there is no error.
			if err := t.fd.SetWriteDeadline(time.Now().Add(discWriteTimeout)); err == nil {
				SendItems(t.rw, discMsg, r)
			}
		}
	}
	t.fd.Close()
}

func (t *rlpx) doProtoHandshake(our *protoHandshake) (their *protoHandshake, err error) {
	// Writing our handshake happens concurrently, we prefer
	// returning the handshake read error. If the remote side
	// disconnects us early with a valid reason, we should return it
	// as the error so it can be tracked elsewhere.
	werr := make(chan error, 1)
	go func() { werr <- Send(t.rw, handshakeMsg, our) }()
	if their, err = readProtocolHandshake(t.rw); err != nil {
		<-werr // make sure the write terminates too
		return nil, err
	}
	if err := <-werr; err != nil {
		return nil, fmt.Errorf("write error: %v", err)
	}
	// If the protocol version supports Snappy encoding, upgrade immediately
	t.rw.snappy = their.Version >= snappyProtocolVersion

	return their, nil
}

func readProtocolHandshake(rw MsgReader) (*protoHandshake, error) {
	msg, err := rw.ReadMsg()
	if err != nil {
		return nil, err
	}
	if msg.Size > baseProtocolMaxMsgSize {
		return nil, fmt.Errorf("message too big")
	}
	if msg.Code == discMsg {
		// Disconnect before protocol handshake is valid according to the
		// spec and we send it ourself if the post-handshake checks fail.
		// We can't return the reason directly, though, because it is echoed
		// back otherwise. Wrap it in a string instead.
		var reason [1]DiscReason
		rlp.Decode(msg.Payload, &reason)
		return nil, reason[0]
	}
	if msg.Code != handshakeMsg {
		return nil, fmt.Errorf("expected handshake, got %x", msg.Code)
	}
	var hs protoHandshake
	if err := msg.Decode(&hs); err != nil {
		return nil, err
	}
	if len(hs.ID) != 64 || !bitutil.TestBytes(hs.ID) {
		return nil, DiscInvalidIdentity
	}
	return &hs, nil
}

// doEncHandshake runs the protocol handshake using authenticated
// messages. the protocol handshake is the first authenticated message
// and also verifies whether the encryption handshake 'worked' and the
// remote side actually provided the right public key.
func (t *rlpx) doEncHandshake(prv *ecdsa.PrivateKey, dial *ecdsa.PublicKey) (*ecdsa.PublicKey, error) {
	var (
		sec secrets
		err error
	)
	if dial == nil {
		sec, err = t.receiverEncHandshake(t.fd, prv)
	} else {
		sec, err = t.initiatorEncHandshake(t.fd, prv, dial)
	}

	if err != nil {
		if dial == nil {
			fmt.Println("doEncHandshake ", err)
		} else {
			fmt.Println("doEncHandshake ", err, " ", hex.EncodeToString(crypto.FromECDSAPub(dial)))
		}

		return nil, err
	}
	t.wmu.Lock()
	t.rw = newRLPXFrameRW(t.fd, sec)
	t.wmu.Unlock()
	return sec.Remote, nil
}

// encHandshake contains the state of the encryption handshake.
type encHandshake struct {
	CertSize             uint16
	initiator            bool
	remote               *ecdsa.PublicKey  // remote-pubk
	initNonce, respNonce []byte            // nonce
	randomPrivKey        *ecdsa.PrivateKey // ecdhe-random
	remoteRandomPub      *ecdsa.PublicKey  // ecdhe-random-pubk

}

// secrets represents the connection secrets
// which are negotiated during the encryption handshake.
type secrets struct {
	Remote *ecdsa.PublicKey

	AES, MAC              []byte
	EgressMAC, IngressMAC hash.Hash
	Token                 []byte
}

// RLPx v4 handshake auth (defined in EIP-8).
type authMsgV4 struct {
	Signature       [sigLen]byte
	InitiatorPubkey [pubLen]byte
	Nonce           [shaLen]byte
	Version         uint

	CertSize uint16 `rlp:"-"`
}

// RLPx v4 handshake response (defined in EIP-8).
type authRespV4 struct {
	RandomPubkey [pubLen]byte
	Nonce        [shaLen]byte
	Version      uint

	CertSize uint16 `rlp:"-"`
}

// secrets is called after the handshake is completed.
// It extracts the connection secrets from the handshake values.
func (h *encHandshake) secrets(auth, authResp []byte) (secrets, error) {
	ecdheSecret, err := crypto.GenerateShared(h.randomPrivKey, h.remoteRandomPub, sskLen, sskLen)
	if err != nil {
		return secrets{}, err
	}

	// derive base secrets from ephemeral key agreement
	sharedSecret := crypto.Keccak256(ecdheSecret, crypto.Keccak256(h.respNonce, h.initNonce))
	aesSecret := crypto.Keccak256(ecdheSecret, sharedSecret)
	s := secrets{
		Remote: h.remote,
		AES:    aesSecret,
		MAC:    crypto.Keccak256(ecdheSecret, aesSecret),
	}
	// setup sha3 instances for the MACs
	mac1 := sha3.NewLegacyKeccak256()
	mac1.Write(xor(s.MAC, h.respNonce))
	mac1.Write(auth)
	mac2 := sha3.NewLegacyKeccak256()
	mac2.Write(xor(s.MAC, h.initNonce))
	mac2.Write(authResp)
	if h.initiator {
		s.EgressMAC, s.IngressMAC = mac1, mac2
	} else {
		s.EgressMAC, s.IngressMAC = mac2, mac1
	}

	return s, nil
}

// staticSharedSecret returns the static shared secret, the result
// of key agreement between the local and remote static node key.
func (h *encHandshake) staticSharedSecret(prv *ecdsa.PrivateKey) ([]byte, error) {
	return crypto.GenerateShared(prv, h.remote, sskLen, sskLen)
}

// initiatorEncHandshake negotiates a session token on conn.
// it should be called on the dialing side of the connection.
//
// prv is the local client's private key.
func (t *rlpx) initiatorEncHandshake(conn io.ReadWriter, prv *ecdsa.PrivateKey, remote *ecdsa.PublicKey) (s secrets, err error) {
	size := 0
	if t.cm != nil {
		size = len(t.cm.cert)
	}
	h := &encHandshake{initiator: true, remote: remote, CertSize: uint16(size)}
	authMsg, err := h.makeAuthMsg(prv)
	if err != nil {
		return s, err
	}
	authPacket, err := authMsg.sealPlain(h)
	if err != nil {
		return s, err
	}
	if _, err = conn.Write(authPacket); err != nil {
		return s, err
	}

	authRespMsg := new(authRespV4)
	authRespPacket, err := readHandshakeMsg(authRespMsg, encAuthRespLen, prv, conn)
	if err != nil {
		return s, err
	}
	if err := h.handleAuthResp(authRespMsg); err != nil {
		return s, err
	}

	if t.cm != nil && len(t.cm.cert) != 0 {
		if authRespMsg.CertSize == 0 {
			return s, errors.New("initiator remote cert size  equal 0")
		}
		if uint16(len(t.cm.cert)) != authRespMsg.CertSize {
			return s, errors.New("remote cert size error")
		}
		fmt.Println("initiator packet ", len(authPacket), "", len(authRespPacket))
		fmt.Println("initiator ", len(t.cm.cert), " ", authRespMsg.CertSize, " ", hex.EncodeToString(t.cm.cert))
		if _, err = conn.Write(t.cm.cert); err != nil {
			return s, err
		}
		buf := make([]byte, authRespMsg.CertSize)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return s, err
		}

		if err = t.cm.list.VerifyCert(buf); err != nil {
			return s, err
		}
		pub, err := crypto.FromCertBytesToPubKey(buf)
		if err != nil {
			return s, err
		}

		if !reflect.DeepEqual(pub, h.remote) {
			fmt.Println("pub", hex.EncodeToString(crypto.FromECDSAPub(pub)), " remote ", hex.EncodeToString(crypto.FromECDSAPub(h.remote)))
			return s, errors.New("cert not match private key")
		}
	}

	return h.secrets(authPacket, authRespPacket)
}

// makeAuthMsg creates the initiator handshake message.
func (h *encHandshake) makeAuthMsg(prv *ecdsa.PrivateKey) (*authMsgV4, error) {
	// Generate random initiator nonce.
	h.initNonce = make([]byte, shaLen)
	_, err := rand.Read(h.initNonce)
	if err != nil {
		return nil, err
	}
	// Generate random keypair to for ECDH.
	h.randomPrivKey, err = crypto.GenerateKey()
	if err != nil {
		return nil, err
	}

	// Sign known message: static-shared-secret ^ nonce
	token, err := h.staticSharedSecret(prv)
	if err != nil {
		return nil, err
	}
	signed := xor(token, h.initNonce)
	signature, err := crypto.Sign(signed, h.randomPrivKey)

	if err != nil {
		return nil, err
	}

	msg := new(authMsgV4)
	copy(msg.Signature[:], signature)
	copy(msg.InitiatorPubkey[:], crypto.FromECDSAPub(&prv.PublicKey)[1:])

	copy(msg.Nonce[:], h.initNonce)
	msg.Version = TaiRLPXVersion
	msg.CertSize = h.CertSize
	return msg, nil
}

func (h *encHandshake) handleAuthResp(msg *authRespV4) (err error) {
	h.respNonce = msg.Nonce[:]
	h.remoteRandomPub, err = importPublicKey(msg.RandomPubkey[:])
	if msg.Version != TaiRLPXVersion {
		return fmt.Errorf("enc AuthResp %d version error", msg.Version)
	}
	return err
}

// receiverEncHandshake negotiates a session token on conn.
// it should be called on the listening side of the connection.
//
// prv is the local client's private key.
func (t *rlpx) receiverEncHandshake(conn io.ReadWriter, prv *ecdsa.PrivateKey) (s secrets, err error) {

	authMsg := new(authMsgV4)
	authPacket, err := readHandshakeMsg(authMsg, encAuthMsgLen, prv, conn)
	if err != nil {
		return s, err
	}
	h := new(encHandshake)
	if t.cm != nil {
		h.CertSize = uint16(len(t.cm.cert))
	}
	if err := h.handleAuthMsg(authMsg, prv); err != nil {
		return s, err
	}

	find := false
	if t.cm != nil && len(t.cm.cert) != 0 {
		if authMsg.CertSize == 0 {
			return s, errors.New("receiver remote cert size  equal 0")
		}
		if uint16(len(t.cm.cert)) != authMsg.CertSize {
			return s, errors.New("remote cert size error")
		}
		find = true
	}

	authRespMsg, err := h.makeAuthResp()
	if err != nil {
		return s, err
	}

	authRespPacket, err := authRespMsg.sealPlain(h)
	if err != nil {
		return s, err
	}
	if _, err = conn.Write(authRespPacket); err != nil {
		return s, err
	}
	if authMsg.Version != TaiRLPXVersion {
		return s, fmt.Errorf("enc handshake %d version error", authMsg.Version)
	}
	if find {
		buf := make([]byte, authMsg.CertSize)
		if _, err := io.ReadFull(conn, buf); err != nil {
			return s, err
		}

		fmt.Println("receiver ", len(t.cm.cert), " ", authRespMsg.CertSize, " ", hex.EncodeToString(t.cm.cert))
		fmt.Println("receiver packet ", len(authPacket), "", len(authRespPacket), " cert ", authMsg.CertSize, " ", len(buf), " ", hex.EncodeToString(buf))
		if err = t.cm.list.VerifyCert(buf); err != nil {
			return s, err
		}
		pub, err := crypto.FromCertBytesToPubKey(buf)
		if err != nil {
			return s, err
		}
		if !reflect.DeepEqual(pub, h.remote) {
			return s, errors.New("cert not match private key")
		}

		if _, err = conn.Write(t.cm.cert); err != nil {
			return s, err
		}
	}
	return h.secrets(authPacket, authRespPacket)
}

func (h *encHandshake) handleAuthMsg(msg *authMsgV4, prv *ecdsa.PrivateKey) error {
	// Import the remote identity.
	rpub, err := importPublicKey(msg.InitiatorPubkey[:])
	if err != nil {
		return err
	}
	h.initNonce = msg.Nonce[:]
	h.remote = rpub

	// Generate random keypair for ECDH.
	// If a private key is already set, use it instead of generating one (for testing).
	if h.randomPrivKey == nil {
		h.randomPrivKey, err = crypto.GenerateKey()
		if err != nil {
			return err
		}
	}

	// Check the signature.
	token, err := h.staticSharedSecret(prv)
	if err != nil {
		return err
	}
	signedMsg := xor(token, h.initNonce)
	h.remoteRandomPub, err = crypto.SigToPub(signedMsg, msg.Signature[:])
	if err != nil {
		return err
	}
	return nil
}

func (h *encHandshake) makeAuthResp() (msg *authRespV4, err error) {
	// Generate random nonce.
	h.respNonce = make([]byte, shaLen)
	if _, err = rand.Read(h.respNonce); err != nil {
		return nil, err
	}

	msg = new(authRespV4)
	copy(msg.Nonce[:], h.respNonce)
	copy(msg.RandomPubkey[:], exportPubkey(&h.randomPrivKey.PublicKey))
	msg.Version = TaiRLPXVersion
	msg.CertSize = h.CertSize
	return msg, nil
}

func (msg *authMsgV4) sealPlain(h *encHandshake) ([]byte, error) {
	data, err := rlp.EncodeToBytes(msg)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, msg.CertSize)
	return crypto.Encrypt(h.remote, append(buf, data...), nil, nil)
}

func (msg *authMsgV4) decodePlain(input []byte) {
	if err := rlp.Decode(bytes.NewReader(input), msg); err != nil {
		panic("authRespV4 decode error")
	}
}

func (msg *authMsgV4) setCertSize(size uint16) {
	msg.CertSize = size
}

func (msg *authRespV4) sealPlain(hs *encHandshake) ([]byte, error) {
	data, err := rlp.EncodeToBytes(msg)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, msg.CertSize)
	return crypto.Encrypt(hs.remote, append(buf, data...), nil, nil)

}

func (msg *authRespV4) decodePlain(input []byte) {
	if err := rlp.Decode(bytes.NewReader(input), msg); err != nil {
		panic("authRespV4 decode error")
	}
}

func (msg *authRespV4) setCertSize(size uint16) {
	msg.CertSize = size
}

type plainDecoder interface {
	decodePlain([]byte)
	setCertSize(uint16)
}

func readHandshakeMsg(msg plainDecoder, plainSize int, prv *ecdsa.PrivateKey, r io.Reader) ([]byte, error) {

	buf := make([]byte, plainSize)
	if _, err := io.ReadFull(r, buf); err != nil {
		return buf, err
	}

	if dec, err := crypto.Decrypt(prv, buf, nil, nil); err == nil {
		prefix := dec[:2]
		size := binary.BigEndian.Uint16(prefix)
		msg.decodePlain(dec[2:])
		msg.setCertSize(size)
		return buf, nil
	}
	return nil, errors.New("Decrypt error")
}

// importPublicKey unmarshals 512 bit public keys.
func importPublicKey(pubKey []byte) (*ecdsa.PublicKey, error) {

	var pubKey65 []byte

	switch len(pubKey) {
	case 64:
		// add 'uncompressed key' flag
		pubKey65 = append([]byte{0x04}, pubKey...)
	case 65:
		pubKey65 = pubKey
	default:
		return nil, fmt.Errorf("invalid public key length %v (expect 64/65)", len(pubKey))
	}
	// TODO: fewer pointless conversions
	pub, err := crypto.UnmarshalPubkey(pubKey65)

	if err != nil {
		return nil, err
	}

	return pub, nil
}

func exportPubkey(pub *ecdsa.PublicKey) []byte {
	if pub == nil {
		panic("nil pubkey")
	}
	return elliptic.Marshal(pub.Curve, pub.X, pub.Y)[1:]
}

func xor(one, other []byte) (xor []byte) {
	xor = make([]byte, len(one))
	for i := 0; i < len(one); i++ {
		xor[i] = one[i] ^ other[i]
	}
	return xor
}

var (
	// this is used in place of actual frame header data.
	// TODO: replace this when Msg contains the protocol type code.
	zeroHeader = []byte{0xC2, 0x80, 0x80}
	// sixteen zero bytes
	zero16 = make([]byte, 16)
)

// rlpxFrameRW implements a simplified version of RLPx framing.
// chunked messages are not supported and all headers are equal to
// zeroHeader.
//
// rlpxFrameRW is not safe for concurrent use from multiple goroutines.
type rlpxFrameRW struct {
	conn io.ReadWriter
	enc  cipher.Stream
	dec  cipher.Stream

	macCipher  cipher.Block
	egressMAC  hash.Hash
	ingressMAC hash.Hash

	snappy bool
}

func newRLPXFrameRW(conn io.ReadWriter, s secrets) *rlpxFrameRW {
	macc, err := aes.NewCipher(s.MAC)
	if err != nil {
		panic("invalid MAC secret: " + err.Error())
	}
	encc, err := aes.NewCipher(s.AES)
	if err != nil {
		panic("invalid AES secret: " + err.Error())
	}
	// we use an all-zeroes IV for AES because the key used
	// for encryption is ephemeral.
	iv := make([]byte, encc.BlockSize())
	return &rlpxFrameRW{
		conn:       conn,
		enc:        cipher.NewCTR(encc, iv),
		dec:        cipher.NewCTR(encc, iv),
		macCipher:  macc,
		egressMAC:  s.EgressMAC,
		ingressMAC: s.IngressMAC,
	}
}

func (rw *rlpxFrameRW) WriteMsg(msg Msg) error {
	ptype, _ := rlp.EncodeToBytes(msg.Code)

	// if snappy is enabled, compress message now
	if rw.snappy {
		if msg.Size > maxUint24 {
			return errPlainMessageTooLarge
		}
		payload, _ := ioutil.ReadAll(msg.Payload)
		payload = snappy.Encode(nil, payload)

		msg.Payload = bytes.NewReader(payload)
		msg.Size = uint32(len(payload))
	}
	// write header
	headbuf := make([]byte, 32)
	fsize := uint32(len(ptype)) + msg.Size
	if fsize > maxUint24 {
		return errors.New("message size overflows uint24")
	}
	putInt24(fsize, headbuf) // TODO: check overflow
	copy(headbuf[3:], zeroHeader)
	rw.enc.XORKeyStream(headbuf[:16], headbuf[:16]) // first half is now encrypted

	// write header MAC
	copy(headbuf[16:], updateMAC(rw.egressMAC, rw.macCipher, headbuf[:16]))
	if _, err := rw.conn.Write(headbuf); err != nil {
		return err
	}

	// write encrypted frame, updating the egress MAC hash with
	// the data written to conn.
	tee := cipher.StreamWriter{S: rw.enc, W: io.MultiWriter(rw.conn, rw.egressMAC)}
	if _, err := tee.Write(ptype); err != nil {
		return err
	}
	if _, err := io.Copy(tee, msg.Payload); err != nil {
		return err
	}
	if padding := fsize % 16; padding > 0 {
		if _, err := tee.Write(zero16[:16-padding]); err != nil {
			return err
		}
	}

	// write frame MAC. egress MAC hash is up to date because
	// frame content was written to it as well.
	fmacseed := rw.egressMAC.Sum(nil)
	mac := updateMAC(rw.egressMAC, rw.macCipher, fmacseed)
	_, err := rw.conn.Write(mac)
	log.Trace("RLPX write msg", "code", msg.Code, "size", msg.Size, "snappy", rw.snappy, "ptype", len(ptype), "mac", len(mac), "fmacseed", len(fmacseed))
	return err
}

func (rw *rlpxFrameRW) ReadMsg() (msg Msg, err error) {
	// read the header
	headbuf := make([]byte, 32)
	if _, err := io.ReadFull(rw.conn, headbuf); err != nil {
		return msg, err
	}
	// verify header mac
	shouldMAC := updateMAC(rw.ingressMAC, rw.macCipher, headbuf[:16])
	if !hmac.Equal(shouldMAC, headbuf[16:]) {
		return msg, errors.New("bad header MAC")
	}
	rw.dec.XORKeyStream(headbuf[:16], headbuf[:16]) // first half is now decrypted
	fsize := readInt24(headbuf)
	// ignore protocol type for now

	// read the frame content
	var rsize = fsize // frame size rounded up to 16 byte boundary
	if padding := fsize % 16; padding > 0 {
		rsize += 16 - padding
	}
	framebuf := make([]byte, rsize)
	if _, err := io.ReadFull(rw.conn, framebuf); err != nil {
		return msg, err
	}

	// read and validate frame MAC. we can re-use headbuf for that.
	rw.ingressMAC.Write(framebuf)
	fmacseed := rw.ingressMAC.Sum(nil)
	if _, err := io.ReadFull(rw.conn, headbuf[:16]); err != nil {
		return msg, err
	}
	shouldMAC = updateMAC(rw.ingressMAC, rw.macCipher, fmacseed)
	if !hmac.Equal(shouldMAC, headbuf[:16]) {
		return msg, errors.New("bad frame MAC")
	}

	// decrypt frame content
	rw.dec.XORKeyStream(framebuf, framebuf)

	// decode message code
	content := bytes.NewReader(framebuf[:fsize])
	if err := rlp.Decode(content, &msg.Code); err != nil {
		return msg, err
	}
	msg.Size = uint32(content.Len())
	msg.Payload = content

	// if snappy is enabled, verify and decompress message
	if rw.snappy {
		payload, err := ioutil.ReadAll(msg.Payload)
		if err != nil {
			return msg, err
		}
		size, err := snappy.DecodedLen(payload)
		if err != nil {
			return msg, err
		}
		if size > int(maxUint24) {
			return msg, errPlainMessageTooLarge
		}
		payload, err = snappy.Decode(nil, payload)
		if err != nil {
			return msg, err
		}
		msg.Size, msg.Payload = uint32(size), bytes.NewReader(payload)
	}
	return msg, nil
}

// updateMAC reseeds the given hash with encrypted seed.
// it returns the first 16 bytes of the hash sum after seeding.
func updateMAC(mac hash.Hash, block cipher.Block, seed []byte) []byte {
	aesbuf := make([]byte, aes.BlockSize)
	block.Encrypt(aesbuf, mac.Sum(nil))
	for i := range aesbuf {
		aesbuf[i] ^= seed[i]
	}
	mac.Write(aesbuf)
	return mac.Sum(nil)[:16]
}

func readInt24(b []byte) uint32 {
	return uint32(b[2]) | uint32(b[1])<<8 | uint32(b[0])<<16
}

func putInt24(v uint32, b []byte) {
	b[0] = byte(v >> 16)
	b[1] = byte(v >> 8)
	b[2] = byte(v)
}
