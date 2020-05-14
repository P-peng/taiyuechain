// Copyright 2014 The go-ethereum Authors
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

package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/taiyuechain/taiyuechain/common"
	"github.com/taiyuechain/taiyuechain/common/math"
	"github.com/taiyuechain/taiyuechain/crypto/ecies"
	"github.com/taiyuechain/taiyuechain/crypto/gm/sm2"
	"github.com/taiyuechain/taiyuechain/crypto/gm/sm3"
	"github.com/taiyuechain/taiyuechain/rlp"
	"golang.org/x/crypto/sha3"
	"hash"
	"io"
	"io/ioutil"
	"math/big"
	"os"
)

//SignatureLength indicates the byte length required to carry a signature with recovery id.
const SignatureLength = 64 + 1 // 64 bytes ECDSA signature + 1 byte recovery id

// RecoveryIDOffset points to the byte offset within the signature that contains the recovery id.
const RecoveryIDOffset = 64

// DigestLength sets the signature digest exact length
const DigestLength = 32

var (
	secp256k1N, _  = new(big.Int).SetString("fffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141", 16)
	secp256k1halfN = new(big.Int).Div(secp256k1N, big.NewInt(2))
)

var errInvalidPubkey = errors.New("invalid public key")

// Keccak256 calculates and returns the Keccak256 hash of the input data.

func Keccak256(data ...[]byte) []byte {
	if CryptoType == CRYPTO_P256_SH3_AES {
		d := sha3.NewLegacyKeccak256()
		for _, b := range data {
			d.Write(b)
		}
		return d.Sum(nil)
	}
	if CryptoType == CRYPTO_SM2_SM3_SM4 {
		d := sm3.New()
		for _, b := range data {
			d.Write(b)
		}
		return d.Sum(nil)
	}
	return nil
}

// Keccak256Hash calculates and returns the Keccak256 hash of the input data,
// converting it to an internal Hash data structure.
func Keccak256Hash(data ...[]byte) (h common.Hash) {
	if CryptoType == CRYPTO_P256_SH3_AES {
		d := sha3.NewLegacyKeccak256()
		for _, b := range data {
			d.Write(b)
		}
		d.Sum(h[:0])
		return h
	}
	if CryptoType == CRYPTO_SM2_SM3_SM4 {
		d := sm3.New()
		for _, b := range data {
			d.Write(b)
		}
		d.Sum(h[:0])
		return h
	}
	return h
}

// Keccak512 calculates and returns the Keccak512 hash of the input data.
func Keccak512(data ...[]byte) []byte {
	d := sha3.NewLegacyKeccak512()
	for _, b := range data {
		d.Write(b)
	}
	return d.Sum(nil)
}

// CreateAddress creates an ethereum address given the bytes and the nonce
func CreateAddress(b common.Address, nonce uint64) common.Address {
	data, _ := rlp.EncodeToBytes([]interface{}{b, nonce})
	return common.BytesToAddress(Keccak256(data)[12:])
}

// CreateAddress2 creates an ethereum address given the address bytes, initial
// contract code hash and a salt.
func CreateAddress2(b common.Address, salt [32]byte, inithash []byte) common.Address {
	return common.BytesToAddress(Keccak256([]byte{0xff}, b.Bytes(), salt[:], inithash)[12:])
}

// ToECDSA creates a private key with the given D value.
func ToECDSA(d []byte) (*ecdsa.PrivateKey, error) {
	if CryptoType == CRYPTO_P256_SH3_AES {
		ecdsapri, err := toECDSA(elliptic.P256(), d, true)
		if err != nil {
			return nil, err
		}
		return ecdsapri, nil
	}
	//guomi
	if CryptoType == CRYPTO_SM2_SM3_SM4 {
		ecdsapri, err := toECDSA(sm2.P256Sm2(), d, true)
		if err != nil {
			return nil, err
		}
		return ecdsapri, nil
	}
	//guoji S256
	if CryptoType == CRYPTO_S256_SH3_AES {
		ecdsapri, err := toECDSA(S256(), d, true)
		if err != nil {
			return nil, err
		}
		return ecdsapri, nil
	}
	return nil, nil
}
func ToECDSAUnsafe(d []byte) *ecdsa.PrivateKey {
	if CryptoType == CRYPTO_P256_SH3_AES {
		ecdsapri, _ := toECDSA(elliptic.P256(), d, true)
		return ecdsapri
	}
	//guomi
	if CryptoType == CRYPTO_SM2_SM3_SM4 {
		ecdsapri, _ := toECDSA(sm2.P256Sm2(), d, true)
		return ecdsapri
	}
	//guoji S256
	if CryptoType == CRYPTO_S256_SH3_AES {
		ecdsapri, _ := toECDSA(S256(), d, true)

		return ecdsapri
	}
	return nil
}

// toECDSA creates a private key with the given D value. The strict parameter
// controls whether the key's length should be enforced at the curve size or
// it can also accept legacy encodings (0 prefixes).
func toECDSA(curve elliptic.Curve, d []byte, strict bool) (*ecdsa.PrivateKey, error) {
	priv := new(ecdsa.PrivateKey)
	//priv.PublicKey.Curve = S256()
	priv.PublicKey.Curve = curve
	if strict && 8*len(d) != priv.Params().BitSize {
		return nil, fmt.Errorf("invalid length, need %d bits", priv.Params().BitSize)
	}
	priv.D = new(big.Int).SetBytes(d)

	// The priv.D must < N
	if priv.D.Cmp(secp256k1N) >= 0 {
		return nil, fmt.Errorf("invalid private key, >=N")
	}
	// The priv.D must not be zero or negative.
	if priv.D.Sign() <= 0 {
		return nil, fmt.Errorf("invalid private key, zero or negative")
	}

	priv.PublicKey.X, priv.PublicKey.Y = priv.PublicKey.Curve.ScalarBaseMult(d)
	if priv.PublicKey.X == nil {
		return nil, errors.New("invalid private key")
	}
	return priv, nil
}

// FromECDSA exports a private key into a binary dump.
func FromECDSA(priv *ecdsa.PrivateKey) []byte {
	if priv == nil {
		return nil
	}
	return math.PaddedBigBytes(priv.D, priv.Params().BitSize/8)
}
func UnmarshalPubkey(pub []byte) (*ecdsa.PublicKey, error) {
	if CryptoType == CRYPTO_P256_SH3_AES {
		x, y := elliptic.Unmarshal(elliptic.P256(), pub)
		if x == nil {
			return nil, errInvalidPubkey
		}
		return &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}, nil
	}
	//guomi
	if CryptoType == CRYPTO_SM2_SM3_SM4 {
		//ecdsapri, _ := toECDSA(sm2.P256Sm2(),d,true)
		x, y := elliptic.Unmarshal(sm2.P256Sm2(), pub)
		if x == nil {
			return nil, errInvalidPubkey
		}
		return &ecdsa.PublicKey{Curve: sm2.P256Sm2(), X: x, Y: y}, nil
	}
	//guoji S256
	if CryptoType == CRYPTO_S256_SH3_AES {
		x, y := elliptic.Unmarshal(S256(), pub)
		if x == nil {
			return nil, errInvalidPubkey
		}
		return &ecdsa.PublicKey{Curve: S256(), X: x, Y: y}, nil
	}
	return nil, nil
}

func FromECDSAPub(pub *ecdsa.PublicKey) []byte {
	if CryptoType == CRYPTO_P256_SH3_AES {
		if pub == nil || pub.X == nil || pub.Y == nil {
			return nil
		}
		return elliptic.Marshal(elliptic.P256(), pub.X, pub.Y)
	}
	//guomi
	if CryptoType == CRYPTO_SM2_SM3_SM4 {
		if pub == nil || pub.X == nil || pub.Y == nil {
			return nil
		}
		return elliptic.Marshal(sm2.P256Sm2(), pub.X, pub.Y)
	}
	//guoji S256
	if CryptoType == CRYPTO_S256_SH3_AES {
		if pub == nil || pub.X == nil || pub.Y == nil {
			return nil
		}
		return elliptic.Marshal(S256(), pub.X, pub.Y)
	}
	return nil
}

// HexToECDSA parses a secp256k1 private key.
func HexToECDSA(hexkey string) (*ecdsa.PrivateKey, error) {
	b, err := hex.DecodeString(hexkey)
	if err != nil {
		return nil, errors.New("invalid hex string")
	}
	return ToECDSA(b)
}

// LoadECDSA loads a secp256k1 private key from the given file.
func LoadECDSA(file string) (*ecdsa.PrivateKey, error) {
	buf := make([]byte, 64)
	fd, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer fd.Close()
	if _, err := io.ReadFull(fd, buf); err != nil {
		return nil, err
	}

	key, err := hex.DecodeString(string(buf))
	if err != nil {
		return nil, err
	}
	return ToECDSA(key)
}

// SaveECDSA saves a secp256k1 private key to the given file with
// restrictive permissions. The key data is saved hex-encoded.
func SaveECDSA(file string, key *ecdsa.PrivateKey) error {
	k := hex.EncodeToString(FromECDSA(key))
	return ioutil.WriteFile(file, []byte(k), 0600)
}
func GenerateKey() (*ecdsa.PrivateKey, error) {
	switch CryptoType {
	//guoji P256
	case CRYPTO_P256_SH3_AES:
		ecdsapri, err := ecies.GenerateKey(rand.Reader, elliptic.P256(), nil)
		if err != nil {
			return nil, err
		}
		return (ecdsapri.ExportECDSA()), nil
	//	guomi
	case CRYPTO_SM2_SM3_SM4:
		smpri, _ := ecies.GenerateKey(rand.Reader, sm2.P256Sm2(), nil)
		return (smpri.ExportECDSA()), nil
	//	guoji S256
	case CRYPTO_S256_SH3_AES:
		eciespri, _ := ecies.GenerateKey(rand.Reader, S256(), nil)
		return (eciespri.ExportECDSA()), nil
	}
	return nil, nil
}

// ValidateSignatureValues verifies whether the signature values are valid with
// the given chain rules. The v value is assumed to be either 0 or 1.
func ValidateSignatureValues(v byte, r, s *big.Int, homestead bool) bool {
	if r.Cmp(common.Big1) < 0 || s.Cmp(common.Big1) < 0 {
		return false
	}
	// reject upper range of s values (ECDSA malleability)
	// see discussion in secp256k1/libsecp256k1/include/secp256k1.h
	if homestead && s.Cmp(secp256k1halfN) > 0 {
		return false
	}
	// Frontier: allow s to be in full N range
	return r.Cmp(secp256k1N) < 0 && s.Cmp(secp256k1N) < 0 && (v == 0 || v == 1)
}

/*func PubkeyToAddress(p ecdsa.PublicKey) common.Address {
	pubBytes := FromECDSAPubCA(&p)
	return common.BytesToAddress(Keccak256(pubBytes[1:])[12:])
}*/
func PubkeyToAddress(p ecdsa.PublicKey) common.Address {
	pubBytes := FromECDSAPub(&p)
	return common.BytesToAddress(Keccak256(pubBytes[1:])[12:])
}

func Encrypt(pub *ecdsa.PublicKey, m, s1, s2 []byte) (ct []byte, err error) {
	switch CryptoType {
	//guoji P256
	case CRYPTO_P256_SH3_AES:
		return ecies.Encrypt(rand.Reader, ecies.ImportECDSAPublic(pub), m, s1, s2)
	//	guomi
	case CRYPTO_SM2_SM3_SM4:
		return sm2.Encrypt(sm2.ToSm2Publickey(pub), m, sm2.C1C2C3)

	//	guoji S256
	case CRYPTO_S256_SH3_AES:
		return ecies.Encrypt(rand.Reader, ecies.ImportECDSAPublic(pub), m, s1, s2)
	}
	return nil, nil
}
func Decrypt(pri *ecdsa.PrivateKey, c, s1, s2 []byte) (m []byte, err error) {
	switch CryptoType {
	//guoji P256
	case CRYPTO_P256_SH3_AES:
		return ecies.ImportECDSA(pri).Decrypt(c, s1, s2)
	//	guomi
	case CRYPTO_SM2_SM3_SM4:
		return sm2.Decrypt(sm2.ToSm2privatekey(pri), c, sm2.C1C2C3)

	//	guoji S256
	case CRYPTO_S256_SH3_AES:
		return ecies.ImportECDSA(pri).Decrypt(c, s1, s2)
	}
	return nil, nil
}
func GenerateShared(pri *ecdsa.PrivateKey, pub *ecdsa.PublicKey, skLen, macLen int) (sk []byte, err error) {
	switch CryptoType {
	//guoji P256
	case CRYPTO_P256_SH3_AES:
		return ecies.ImportECDSA(pri).GenerateShared(ecies.ImportECDSAPublic(pub), skLen, macLen)
	//	guomi
	case CRYPTO_SM2_SM3_SM4:
		return sm2.ToSm2privatekey(pri).GenerateShared(sm2.ToSm2Publickey(pub), skLen, macLen)

	//	guoji S256
	case CRYPTO_S256_SH3_AES:
		return ecies.ImportECDSA(pri).GenerateShared(ecies.ImportECDSAPublic(pub), skLen, macLen)
	}
	return nil, nil
}
func zeroBytes(bytes []byte) {
	for i := range bytes {
		bytes[i] = 0
	}
}

/*
hash method
*/
func Hash256(auth, s, h []byte) hash.Hash {
	switch CryptoType {
	case CRYPTO_P256_SH3_AES:
		mac := sha3.NewLegacyKeccak256()
		mac.Write(xor(s, h))
		mac.Write(auth)
		return mac

	case CRYPTO_SM2_SM3_SM4:
		mac := sm3.New()
		mac.Write(xor(s, h))
		mac.Write(auth)
		return mac

	}
	return nil
}
func Hash256Byte(seedBytes, riseedBytes []byte) []byte {
	switch CryptoType {
	case CRYPTO_P256_SH3_AES:
		h := sha256.New()
		h.Write(seedBytes)
		h.Write(riseedBytes)
		return h.Sum(nil)

	case CRYPTO_SM2_SM3_SM4:
		h := sm3.New()
		h.Write(seedBytes)
		h.Write(riseedBytes)
		return h.Sum(nil)
	}
	return nil
}
func Hex(a []byte) string {
	switch CryptoType {
	case CRYPTO_P256_SH3_AES:
		unchecksummed := hex.EncodeToString(a[:])
		sha := sha3.NewLegacyKeccak256()
		sha.Write([]byte(unchecksummed))
		hash := sha.Sum(nil)

		result := []byte(unchecksummed)
		for i := 0; i < len(result); i++ {
			hashByte := hash[i/2]
			if i%2 == 0 {
				hashByte = hashByte >> 4
			} else {
				hashByte &= 0xf
			}
			if result[i] > '9' && hashByte > 7 {
				result[i] -= 32
			}
		}
		return "0x" + string(result)

	case CRYPTO_SM2_SM3_SM4:
		unchecksummed := hex.EncodeToString(a[:])
		s3 := sm3.New()
		s3.Write([]byte(unchecksummed))
		hash := s3.Sum(nil)

		result := []byte(unchecksummed)
		for i := 0; i < len(result); i++ {
			hashByte := hash[i/2]
			if i%2 == 0 {
				hashByte = hashByte >> 4
			} else {
				hashByte &= 0xf
			}
			if result[i] > '9' && hashByte > 7 {
				result[i] -= 32
			}
		}
		return "0x" + string(result)
	}

	return ""
}
func xor(one, other []byte) (xor []byte) {
	xor = make([]byte, len(one))
	for i := 0; i < len(one); i++ {
		xor[i] = one[i] ^ other[i]
	}
	return xor
}
func Double256(b []byte) []byte {
	switch CryptoType {
	case CRYPTO_P256_SH3_AES:
		hasher := sha256.New()
		hasher.Write(b) // nolint: errcheck, gas
		sum := hasher.Sum(nil)
		hasher.Reset()
		hasher.Write(sum) // nolint: errcheck, gas
		return hasher.Sum(nil)
	case CRYPTO_SM2_SM3_SM4:
		hasher := sm3.New()
		hasher.Write(b) // nolint: errcheck, gas
		sum := hasher.Sum(nil)
		hasher.Reset()
		hasher.Write(sum) // nolint: errcheck, gas
		return hasher.Sum(nil)
	}
	return nil
}
func RlpHash(x interface{}) (h common.Hash) {
	switch CryptoType {
	case CRYPTO_P256_SH3_AES:
		hw := sha3.NewLegacyKeccak256()
		rlp.Encode(hw, x)
		hw.Sum(h[:0])
		return h
	case CRYPTO_SM2_SM3_SM4:
		hw := sm3.New()
		rlp.Encode(hw, x)
		hw.Sum(h[:0])
		return h
	}
	return h
}
func NewHash() hash.Hash {
	switch CryptoType {
	case CRYPTO_P256_SH3_AES:
		return sha256.New()
	case CRYPTO_SM2_SM3_SM4:
		return sm3.New()
	}
	return nil
}
