package math

import (
	"encoding/hex"
	"fmt"
	"github.com/taiyuechain/taiyuechain/accounts/abi/bind"
	"github.com/taiyuechain/taiyuechain/accounts/abi/bind/backends"
	taicert "github.com/taiyuechain/taiyuechain/cert"
	"github.com/taiyuechain/taiyuechain/common"
	"github.com/taiyuechain/taiyuechain/core"
	"github.com/taiyuechain/taiyuechain/core/types"
	"github.com/taiyuechain/taiyuechain/crypto"
	"github.com/taiyuechain/taiyuechain/log"
	"github.com/taiyuechain/taiyuechain/params"
	"golang.org/x/crypto/sha3"
	"math/big"
	"os"
	"testing"
)

func init() {
	log.Root().SetHandler(log.LvlFilterHandler(log.LvlTrace, log.StreamHandler(os.Stderr, log.TerminalFormat(false))))
}

var (
	pbft1Name = "pbft1priv"
	pbft1path = "../../../cim/testdata/testcert/" + pbft1Name + ".pem"

	gspec = DefaulGenesisBlock()

	//p2p 1
	priKey, _ = crypto.GenerateKey()
	// p2p 2
	skey1, _ = crypto.GenerateKey()
	// pbft 1
	dkey1, _ = crypto.GenerateKey()
	mAccount = crypto.PubkeyToAddress(priKey.PublicKey)
	saddr1   = crypto.PubkeyToAddress(skey1.PublicKey)
	daddr1   = crypto.PubkeyToAddress(dkey1.PublicKey)

	pbft1Byte, _ = taicert.ReadPemFileByPath(pbft1path)
)

func DefaulGenesisBlock() *core.Genesis {
	i, _ := new(big.Int).SetString("10000000000000000000000", 10)
	key1 := crypto.FromECDSAPub(&dkey1.PublicKey)

	var certList = [][]byte{pbft1Byte}
	coinbase := daddr1

	return &core.Genesis{
		Config:       params.DevnetChainConfig,
		GasLimit:     20971520,
		UseGas:       1,
		IsCoin:   1,
		KindOfCrypto: 3,
		Timestamp:    1537891200,
		Alloc: map[common.Address]types.GenesisAccount{
			mAccount: {Balance: i},
		},
		Committee: []*types.CommitteeMember{
			&types.CommitteeMember{Coinbase: coinbase, Publickey: key1},
		},
		CertList: certList,
	}
}

func TestMath(t *testing.T) {
	contractBackend := backends.NewSimulatedBackend(gspec, 10000000)
	transactOpts := bind.NewKeyedTransactor(priKey, gspec.Config.ChainID)
	// Deploy the ENS registry
	ensAddr, _, _, err := DeployToken(transactOpts, contractBackend)
	if err != nil {
		t.Fatalf("can't DeployContract: %v", err)
	}
	ens, err := NewToken(ensAddr, contractBackend)
	if err != nil {
		t.Fatalf("can't NewContract: %v", err)
	}
	fmt.Println("11111111111111111111111111111111111111111111111111111111111111111111111111111")
	_, err = ens.Add(transactOpts, big.NewInt(50000))
	if err != nil {
		log.Error("Failed to request token transfer", ": %v", err)
	}
	fmt.Println("2222222222222222222222222222222222222222222222222222222222222222222222222222222")

	//fmt.Printf("Transfer pending: 0x%x\n", tx.Hash())
	contractBackend.Commit()
}

func TestMethod(t *testing.T) {
	method := []byte("add(uint256)")
	sig := crypto.Keccak256(method)[:4]
	fmt.Println(" ", hex.EncodeToString(sig))
	d := sha3.NewLegacyKeccak256()
	d.Write(method)
	fmt.Println(" ", hex.EncodeToString(d.Sum(nil)[:4]))
}
