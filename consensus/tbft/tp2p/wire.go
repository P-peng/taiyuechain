package tp2p

import (
	amino "github.com/tendermint/go-amino"
	cryptoAmino "github.com/taiyuechain/taiyuechain/consensus/tbft/crypto/cryptoamino"
)

var cdc = amino.NewCodec()

func init() {
	cryptoAmino.RegisterAmino(cdc)
}
