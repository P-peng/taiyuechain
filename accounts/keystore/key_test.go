package keystore

import (
	"crypto/ecdsa"
	"crypto/x509"
	"fmt"
	"github.com/taiyuechain/taiyuechain/cim"
	"github.com/taiyuechain/taiyuechain/core/types"
	"github.com/taiyuechain/taiyuechain/crypto"
	"github.com/taiyuechain/taiyuechain/params"
	"github.com/taiyuechain/taiyuechain/utils/constant"
	"math/big"
	"testing"
)

var (
	chainID = big.NewInt(11155)
	signer  = types.NewSigner(chainID)
)

func TestP256Sin(t *testing.T) {
	//NewP256Transaction(nonce uint64, to *common.Address, payer *common.Address, amount *big.Int, fee *big.Int, gasLimit uint64, gasPrice *big.Int, data []byte, cert []byte,chainID *big.Int,sig []byte)
	var toPrive, _ = crypto.HexToECDSA("696b0620068602ecdda42ada206f74952d8c305a811599d463b89cfa3ba3bb98")
	var fromPrive, _ = crypto.HexToECDSA("c1094d6cc368fa78f0175974968e9bf3d82216e87a6dfd59328220ac74181f47")

	from := crypto.PubkeyToAddress(fromPrive.PublicKey)
	amount := new(big.Int).SetInt64(0)
	nonce := uint64(1)

	// to

	tocertbyte := cim.CreateCertP256(toPrive)

	toCert, err := x509.ParseCertificate(tocertbyte)
	if err != nil {
		t.Fatalf("ParseCertificate err")
		return
	}
	//fmt.Println(tocert.Version)
	var topubk ecdsa.PublicKey
	switch pub := toCert.PublicKey.(type) {
	case *ecdsa.PublicKey:
		topubk.Curve = pub.Curve
		topubk.X = pub.X
		topubk.Y = pub.Y
	}
	to := crypto.PubkeyToAddress(topubk)
	//fmt.Println("to","is",to)
	// from
	fromcert := cim.CreateCertP256(fromPrive)

	tx := types.NewP256Transaction(nonce, &to, nil, amount, new(big.Int).SetInt64(0), params.TxGas, new(big.Int).SetInt64(0), nil, fromcert, chainID, nil)

	signTx, _ := types.SignTx(tx, signer, fromPrive)

	/*err2 := types.VerfiySignTx(signTx, signer)
	if err2 != nil {
		t.Fatalf("Verfiy err")
	}*/

	//getFrom
	fromCertTx := signTx.Cert()
	fromAddr, err := x509.ParseCertificate(fromCertTx)
	if err != nil {
		t.Fatalf("ParseCertificate err")
		return
	}
	//fmt.Println(tocert.Version)
	var frompubkTx ecdsa.PublicKey
	switch pub := fromAddr.PublicKey.(type) {
	case *ecdsa.PublicKey:
		frompubkTx.Curve = pub.Curve
		frompubkTx.X = pub.X
		frompubkTx.Y = pub.Y
	}

	if from != crypto.PubkeyToAddress(frompubkTx) {
		t.Fatalf("from err")
		return
	}
	fmt.Println(from)
	fmt.Println(crypto.PubkeyToAddress(frompubkTx))

}

/*func TestGMSin(t *testing.T) {
	//NewP256Transaction(nonce uint64, to *common.Address, payer *common.Address, amount *big.Int, fee *big.Int, gasLimit uint64, gasPrice *big.Int, data []byte, cert []byte,chainID *big.Int,sig []byte)
	var toPrive, _ = crypto.HexToECDSA("696b0620068602ecdda42ada206f74952d8c305a811599d463b89cfa3ba3bb98")
	var fromPrive, _ = crypto.HexToECDSA("c1094d6cc368fa78f0175974968e9bf3d82216e87a6dfd59328220ac74181f47")

	if toPrive == nil || fromPrive == nil {
		t.Fatalf("private key sm err")
		return
	}

	frompub := crypto.PrivteToPublickey(*fromPrive)
	from := crypto.GMPubkeyToAddress(*frompub)
	amount := new(big.Int).SetInt64(0)
	nonce := uint64(1)

	toPub := sm2.PrivteToPublickey(*toPrive)

	// to

	tocertbyte := sm2_cert.CreateCertBySMPrivte(toPrive, frompub)

	toCert, err := sm2_cert.ParseCertificate(tocertbyte)
	if err != nil {
		t.Fatalf("ParseCertificate err")
		return
	}
	//fmt.Println(tocert.Version)
	var topubk sm2.PublicKey
	switch pub := toCert.PublicKey.(type) {
	case *sm2.PublicKey:
		topubk.Curve = pub.Curve
		topubk.X = pub.X
		topubk.Y = pub.Y
	}
	to := sm2.GMPubkeyToAddress(topubk)
	//fmt.Println("to","is",to)
	// from
	fromcertbyte := sm2_cert.CreateCertBySMPrivte(toPrive, toPub)

	tx := types.NewP256Transaction(nonce, &to, nil, amount, new(big.Int).SetInt64(0), params.TxGas, new(big.Int).SetInt64(0), nil, fromcertbyte, chainID, nil)

	signTx, _ := types.SignTx(tx, signer, fromPrive)

	err2 := types.VerfiySignTx(signTx, signer)
	if err2 != nil {
		t.Fatalf("Verfiy err")
	}

	//getFrom
	fromCertTx := signTx.Cert()
	fromAddr, err := sm2_cert.ParseCertificateRequest(fromCertTx)
	if err != nil {
		t.Fatalf("ParseCertificate err")
		return
	}

	var frompubkTx sm2.PublicKey
	switch pub := fromAddr.PublicKey.(type) {
	case *sm2.PublicKey:
		frompubkTx.Curve = pub.Curve
		frompubkTx.X = pub.X
		frompubkTx.Y = pub.Y
	}

	if from != sm2.GMPubkeyToAddress(frompubkTx) {
		t.Fatalf("from err")
		return
	}
	fmt.Println(from)
	fmt.Println(sm2.GMPubkeyToAddress(frompubkTx))

}*/
/*func TestGMSin1(t *testing.T) {
	//NewP256Transaction(nonce uint64, to *common.Address, payer *common.Address, amount *big.Int, fee *big.Int, gasLimit uint64, gasPrice *big.Int, data []byte, cert []byte,chainID *big.Int,sig []byte)

	taiCrypto.AsymmetricCryptoType = taiCrypto.ASYMMETRICCRYPTOSM2
	var toPrive1, _ = taiCrypto.HexToTaiPrivateKey("696b0620068602ecdda42ada206f74952d8c305a811599d463b89cfa3ba3bb98")
	var fromPrive1, _ = taiCrypto.HexToTaiPrivateKey("c1094d6cc368fa78f0175974968e9bf3d82216e87a6dfd59328220ac74181f47")
	toPrive := &toPrive1.GmPrivate
	fromPrive := &fromPrive1.GmPrivate
	if toPrive == nil || fromPrive == nil {
		t.Fatalf("private key sm err")
		return
	}
	frompub := sm2.PrivteToPublickey(*fromPrive)
	from := sm2.GMPubkeyToAddress(*frompub)
	amount := new(big.Int).SetInt64(0)
	nonce := uint64(1)

	//toPub := sm2.PrivteToPublickey(*toPrive)

	// to

	tocertbyte := sm2_cert.CreateCertBySMPrivte(toPrive, frompub)

	//toCert,err := x509.ParseCertificate(tocertbyte)
	toCert, err := sm2_cert.ParseCertificateRequest(tocertbyte)
	if err != nil {
		t.Fatalf("ParseCertificate err")
		return
	}
	//fmt.Println(tocert.Version)
	var topubk sm2.PublicKey
	switch pub := toCert.PublicKey.(type) {
	case *sm2.PublicKey:
		topubk.Curve = pub.Curve
		topubk.X = pub.X
		topubk.Y = pub.Y
	}
	to := sm2.GMPubkeyToAddress(topubk)
	//fmt.Println("to","is",to)
	// from
	//fromcertbyte :=sm2_cert.CreateCertBySMPrivte(toPrive,*toPub)
	fromcertbyte := sm2_cert.CreateCertBySMPrivte(fromPrive, frompub)

	tx := types.NewP256Transaction(nonce, &to, nil, amount, new(big.Int).SetInt64(0), params.TxGas, new(big.Int).SetInt64(0), nil, fromcertbyte, chainID, nil)

	signTx, _ := types.SignTxBySM(tx, signer, fromPrive)
	//signTx,_ := types.SignTxBySM(tx,signer,toPrive);

	err2 := types.VerfiySignTxBySM(signTx, signer)
	if err2 != nil {
		t.Fatalf("Verfiy err")
	}

	//getFrom
	fromCertTx := signTx.Cert()
	fromAddr, err := sm2_cert.ParseCertificateRequest(fromCertTx)
	if err != nil {
		t.Fatalf("ParseCertificate err")
		return
	}

	var frompubkTx sm2.PublicKey
	switch pub := fromAddr.PublicKey.(type) {
	case *sm2.PublicKey:
		frompubkTx.Curve = pub.Curve
		frompubkTx.X = pub.X
		frompubkTx.Y = pub.Y
	}

	if from != sm2.GMPubkeyToAddress(frompubkTx) {
		t.Fatalf("from err")
		return
	}
	fmt.Println(from)
	fmt.Println(sm2.GMPubkeyToAddress(frompubkTx))

}*/
