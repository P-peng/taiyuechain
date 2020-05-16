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
	"crypto/ecdsa"
	"crypto/rand"
	"errors"
	"fmt"
	"github.com/taiyuechain/taiyuechain/cim"
	"io/ioutil"
	"net"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/taiyuechain/taiyuechain/crypto"
	"github.com/taiyuechain/taiyuechain/p2p/simulations/pipes"
	"github.com/taiyuechain/taiyuechain/rlp"
	"golang.org/x/crypto/sha3"
)

func TestSharedSecret(t *testing.T) {
	prv0, _ := crypto.GenerateKey()
	pub0 := &prv0.PublicKey
	prv1, _ := crypto.GenerateKey()
	pub1 := &prv1.PublicKey

	ss0, err := crypto.GenerateShared(prv0, pub1, sskLen, sskLen)
	if err != nil {
		return
	}
	ss1, err := crypto.GenerateShared(prv1, pub0, sskLen, sskLen)
	if err != nil {
		return
	}
	t.Logf("Secret:\n%v %x\n%v %x", len(ss0), ss0, len(ss0), ss1)
	if !bytes.Equal(ss0, ss1) {
		t.Errorf("dont match :(")
	}
}

func TestEncHandshake(t *testing.T) {
	for i := 0; i < 10; i++ {
		start := time.Now()
		if err := testEncHandshake(nil); err != nil {
			t.Fatalf("i=%d %v", i, err)
		}
		t.Logf("(without token) %d %v\n", i+1, time.Since(start))
	}
	for i := 0; i < 10; i++ {
		tok := make([]byte, shaLen)
		rand.Reader.Read(tok)
		start := time.Now()
		if err := testEncHandshake(tok); err != nil {
			t.Fatalf("i=%d %v", i, err)
		}
		t.Logf("(with token) %d %v\n", i+1, time.Since(start))
	}
}

func testEncHandshake(token []byte) error {
	type result struct {
		side   string
		pubkey *ecdsa.PublicKey
		err    error
	}
	var (
		prv0, _  = crypto.GenerateKey()
		prv1, _  = crypto.GenerateKey()
		fd0, fd1 = net.Pipe()
		c0, c1   = newRLPX(fd0, nil).(*rlpx), newRLPX(fd1, nil).(*rlpx)
		output   = make(chan result)
	)

	go func() {
		r := result{side: "initiator"}
		defer func() { output <- r }()
		defer fd0.Close()

		r.pubkey, r.err = c0.doEncHandshake(prv0, &prv1.PublicKey)
		if r.err != nil {
			return
		}
		if !reflect.DeepEqual(r.pubkey, &prv1.PublicKey) {
			r.err = fmt.Errorf("remote pubkey mismatch: got %v, want: %v", r.pubkey, &prv1.PublicKey)
		}
	}()
	go func() {
		r := result{side: "receiver"}
		defer func() { output <- r }()
		defer fd1.Close()

		r.pubkey, r.err = c1.doEncHandshake(prv1, nil)
		if r.err != nil {
			return
		}
		if !reflect.DeepEqual(r.pubkey, &prv0.PublicKey) {
			r.err = fmt.Errorf("remote ID mismatch: got %v, want: %v", r.pubkey, &prv0.PublicKey)
		}
	}()

	// wait for results from both sides
	r1, r2 := <-output, <-output
	if r1.err != nil {
		return fmt.Errorf("%s side error: %v", r1.side, r1.err)
	}
	if r2.err != nil {
		return fmt.Errorf("%s side error: %v", r2.side, r2.err)
	}

	// compare derived secrets
	if !reflect.DeepEqual(c0.rw.egressMAC, c1.rw.ingressMAC) {
		return fmt.Errorf("egress mac mismatch:\n c0.rw: %#v\n c1.rw: %#v", c0.rw.egressMAC, c1.rw.ingressMAC)
	}
	if !reflect.DeepEqual(c0.rw.ingressMAC, c1.rw.egressMAC) {
		return fmt.Errorf("ingress mac mismatch:\n c0.rw: %#v\n c1.rw: %#v", c0.rw.ingressMAC, c1.rw.egressMAC)
	}
	if !reflect.DeepEqual(c0.rw.enc, c1.rw.enc) {
		return fmt.Errorf("enc cipher mismatch:\n c0.rw: %#v\n c1.rw: %#v", c0.rw.enc, c1.rw.enc)
	}
	if !reflect.DeepEqual(c0.rw.dec, c1.rw.dec) {
		return fmt.Errorf("dec cipher mismatch:\n c0.rw: %#v\n c1.rw: %#v", c0.rw.dec, c1.rw.dec)
	}
	return nil
}

func TestProtocolHandshake(t *testing.T) {
	var (
		prv0, _ = crypto.HexToECDSA("d5939c73167cd3a815530fd8b4b13f1f5492c1c75e4eafb5c07e8fb7f4b09c7c")
		pub0    = crypto.FromECDSAPub(&prv0.PublicKey)[1:]
		hs0     = &protoHandshake{Version: 3, ID: pub0, Caps: []Cap{{"a", 0}, {"b", 2}}}

		prv1, _ = crypto.HexToECDSA("ea4297749d514cc476fe971a7fe20100cbd29f010864341b3e624e8744d46cec")
		pub1    = crypto.FromECDSAPub(&prv1.PublicKey)[1:]
		hs1     = &protoHandshake{Version: 3, ID: pub1, Caps: []Cap{{"c", 1}, {"d", 3}}}

		wg sync.WaitGroup

		pbft1Name = "pbft1priv"
		pbft2Name = "pbft2priv"
		p2p1Name  = "p2p1cert"
		p2p2Name  = "p2p2cert"

		pbft1path = "../cim/testdata/testcert/" + pbft1Name + ".pem"
		pbft2path = "../cim/testdata/testcert/" + pbft2Name + ".pem"
		p2p1path  = "../cim/testdata/testcert/" + p2p1Name + ".pem"
		p2p2path  = "../cim/testdata/testcert/" + p2p2Name + ".pem"
		CryptoSM2 = uint8(2)
	)

	pbft1Byte, err := crypto.ReadPemFileByPath(pbft1path)
	pbft2Byte, _ := crypto.ReadPemFileByPath(pbft2path)

	p2p1Byte, _ := crypto.ReadPemFileByPath(p2p1path)
	p2p2Byte, _ := crypto.ReadPemFileByPath(p2p2path)

	//new cimList
	cimList := cim.NewCIMList(CryptoSM2)
	cimList.AddCim(cim.CreateCim(pbft1Byte))
	cimList.AddCim(cim.CreateCim(pbft2Byte))
	cm1 := &certManager{list: cimList, cert: p2p1Byte}
	cm2 := &certManager{list: cimList, cert: p2p2Byte}

	fd0, fd1, err := pipes.TCPPipe()
	if err != nil {
		t.Fatal(err)
	}

	wg.Add(2)
	go func() {
		defer wg.Done()
		defer fd0.Close()
		rlpx := newRLPX(fd0, cm1)
		rpubkey, err := rlpx.doEncHandshake(prv0, &prv1.PublicKey)
		if err != nil {
			t.Errorf("dial side enc handshake failed: %v", err)
			return
		}
		if !reflect.DeepEqual(rpubkey, &prv1.PublicKey) {
			t.Errorf("dial side remote pubkey mismatch: got %v, want %v", rpubkey, &prv1.PublicKey)
			return
		}

		phs, err := rlpx.doProtoHandshake(hs0)
		if err != nil {
			t.Errorf("dial side proto handshake error: %v", err)
			return
		}
		phs.Rest = nil
		if !reflect.DeepEqual(phs, hs1) {
			t.Errorf("dial side proto handshake mismatch:\ngot: %s\nwant: %s\n", spew.Sdump(phs), spew.Sdump(hs1))
			return
		}
		rlpx.close(DiscQuitting)
	}()
	go func() {
		defer wg.Done()
		defer fd1.Close()
		rlpx := newRLPX(fd1, cm2)
		rpubkey, err := rlpx.doEncHandshake(prv1, nil)
		if err != nil {
			t.Errorf("listen side enc handshake failed: %v", err)
			return
		}
		if !reflect.DeepEqual(rpubkey, &prv0.PublicKey) {
			t.Errorf("listen side remote pubkey mismatch: got %v, want %v", rpubkey, &prv0.PublicKey)
			return
		}

		phs, err := rlpx.doProtoHandshake(hs1)
		if err != nil {
			t.Errorf("listen side proto handshake error: %v", err)
			return
		}
		phs.Rest = nil
		if !reflect.DeepEqual(phs, hs0) {
			t.Errorf("listen side proto handshake mismatch:\ngot: %s\nwant: %s\n", spew.Sdump(phs), spew.Sdump(hs0))
			return
		}

		if err := ExpectMsg(rlpx, discMsg, []DiscReason{DiscQuitting}); err != nil {
			t.Errorf("error receiving disconnect: %v", err)
		}
	}()
	wg.Wait()
}

func TestProtocolHandshakeErrors(t *testing.T) {
	tests := []struct {
		code uint64
		msg  interface{}
		err  error
	}{
		{
			code: discMsg,
			msg:  []DiscReason{DiscQuitting},
			err:  DiscQuitting,
		},
		{
			code: 0x989898,
			msg:  []byte{1},
			err:  errors.New("expected handshake, got 989898"),
		},
		{
			code: handshakeMsg,
			msg:  make([]byte, baseProtocolMaxMsgSize+2),
			err:  errors.New("message too big"),
		},
		{
			code: handshakeMsg,
			msg:  []byte{1, 2, 3},
			err:  newPeerError(errInvalidMsg, "(code 0) (size 4) rlp: expected input list for p2p.protoHandshake"),
		},
		{
			code: handshakeMsg,
			msg:  &protoHandshake{Version: 3},
			err:  DiscInvalidIdentity,
		},
	}

	for i, test := range tests {
		p1, p2 := MsgPipe()
		go Send(p1, test.code, test.msg)
		_, err := readProtocolHandshake(p2)
		if !reflect.DeepEqual(err, test.err) {
			t.Errorf("test %d: error mismatch: got %q, want %q", i, err, test.err)
		}
	}
}

func TestRLPXFrameFake(t *testing.T) {
	buf := new(bytes.Buffer)
	hash := fakeHash([]byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1})
	rw := newRLPXFrameRW(buf, secrets{
		AES:        crypto.Keccak256(),
		MAC:        crypto.Keccak256(),
		IngressMAC: hash,
		EgressMAC:  hash,
	})

	golden := unhex(`
00828ddae471818bb0bfa6b551d1cb42
01010101010101010101010101010101
ba628a4ba590cb43f7848f41c4382885
01010101010101010101010101010101
`)

	// Check WriteMsg. This puts a message into the buffer.
	if err := Send(rw, 8, []uint{1, 2, 3, 4}); err != nil {
		t.Fatalf("WriteMsg error: %v", err)
	}
	written := buf.Bytes()
	if !bytes.Equal(written, golden) {
		t.Fatalf("output mismatch:\n  got:  %x\n  want: %x", written, golden)
	}

	// Check ReadMsg. It reads the message encoded by WriteMsg, which
	// is equivalent to the golden message above.
	msg, err := rw.ReadMsg()
	if err != nil {
		t.Fatalf("ReadMsg error: %v", err)
	}
	if msg.Size != 5 {
		t.Errorf("msg size mismatch: got %d, want %d", msg.Size, 5)
	}
	if msg.Code != 8 {
		t.Errorf("msg code mismatch: got %d, want %d", msg.Code, 8)
	}
	payload, _ := ioutil.ReadAll(msg.Payload)
	wantPayload := unhex("C401020304")
	if !bytes.Equal(payload, wantPayload) {
		t.Errorf("msg payload mismatch:\ngot  %x\nwant %x", payload, wantPayload)
	}
}

type fakeHash []byte

func (fakeHash) Write(p []byte) (int, error) { return len(p), nil }
func (fakeHash) Reset()                      {}
func (fakeHash) BlockSize() int              { return 0 }

func (h fakeHash) Size() int           { return len(h) }
func (h fakeHash) Sum(b []byte) []byte { return append(b, h...) }

func TestRLPXFrameRW(t *testing.T) {
	var (
		aesSecret      = make([]byte, 16)
		macSecret      = make([]byte, 16)
		egressMACinit  = make([]byte, 32)
		ingressMACinit = make([]byte, 32)
	)
	for _, s := range [][]byte{aesSecret, macSecret, egressMACinit, ingressMACinit} {
		rand.Read(s)
	}
	conn := new(bytes.Buffer)

	s1 := secrets{
		AES:        aesSecret,
		MAC:        macSecret,
		EgressMAC:  sha3.NewLegacyKeccak256(),
		IngressMAC: sha3.NewLegacyKeccak256(),
	}
	s1.EgressMAC.Write(egressMACinit)
	s1.IngressMAC.Write(ingressMACinit)
	rw1 := newRLPXFrameRW(conn, s1)

	s2 := secrets{
		AES:        aesSecret,
		MAC:        macSecret,
		EgressMAC:  sha3.NewLegacyKeccak256(),
		IngressMAC: sha3.NewLegacyKeccak256(),
	}
	s2.EgressMAC.Write(ingressMACinit)
	s2.IngressMAC.Write(egressMACinit)
	rw2 := newRLPXFrameRW(conn, s2)

	// send some messages
	for i := 0; i < 10; i++ {
		// write message into conn buffer
		wmsg := []interface{}{"foo", "bar", strings.Repeat("test", i)}
		err := Send(rw1, uint64(i), wmsg)
		if err != nil {
			t.Fatalf("WriteMsg error (i=%d): %v", i, err)
		}

		// read message that rw1 just wrote
		msg, err := rw2.ReadMsg()
		if err != nil {
			t.Fatalf("ReadMsg error (i=%d): %v", i, err)
		}
		if msg.Code != uint64(i) {
			t.Fatalf("msg code mismatch: got %d, want %d", msg.Code, i)
		}
		payload, _ := ioutil.ReadAll(msg.Payload)
		wantPayload, _ := rlp.EncodeToBytes(wmsg)
		if !bytes.Equal(payload, wantPayload) {
			t.Fatalf("msg payload mismatch:\ngot  %x\nwant %x", payload, wantPayload)
		}
	}
}
