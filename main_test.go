package main

import (
	"math/big"
	"testing"
)

func TestGetAccountDeltasForBlock(t *testing.T) {
	t.Parallel()
	data := []struct {
		block  EthBlock
		deltas map[string]*big.Int
		err    error
	}{
		{
			block: EthBlock{
				Number:        "0x1",
				Miner:         "0x2",
				GasUsed:       "0xabc",
				BaseFeePerGas: "0x12abc3",
				Txs: []EthTx{
					{
						From:     "0x1",
						To:       "0x3",
						Value:    "0x1",
						Gas:      "0x1",
						GasPrice: "0x2",
					},
					{
						From:     "0x3",
						To:       "",
						Value:    "0x0",
						Gas:      "0x1",
						GasPrice: "0x3",
					},
				},
			},
			deltas: map[string]*big.Int{
				"0x1": big.NewInt(-3),
				"0x3": big.NewInt(-2),
			},
		},
	}
	for i, test := range data {
		have, err := getAccountDeltasForBlock(test.block)
		if err != test.err {
			t.Errorf("[Test №%d] expected error '%+v' got '%+v'\n", i, test.err, err)
		}
		for k, v := range test.deltas {
			haveV, ok := have[k]
			if !ok {
				t.Errorf("[Test №%d] expected to have delta for account %s\n", i, k)
				continue
			}
			if haveV.Cmp(v) != 0 {
				t.Errorf("[Test №%d] for account %s expected delta %s, but got %s", i, k, v, haveV)
			}
		}
	}
}
