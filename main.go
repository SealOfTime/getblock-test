package main

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"os"
	"runtime"
	"strconv"
)

var (
	WeiInEth, _ = big.NewFloat(0.0).SetString("1000000000000000000")
)

//Нюансы:
// [X] Учесть газ, который заплатил sender
// [X] Учесть транзакции на развёртывание смарт-контракта
// [X] Учесть, что value транзакции в gwei
// [ ] ? Учитывать ли газ, который получил аккаунт майнера? - пока не буду
// - [ ] ? Как получить baseFee MainNet'а через JSON-RPC api
func main() {
	var (
		nBlocks int64 = 100
		apiKey        = os.Getenv("GETBLOCK_API_KEY")
		err     error
	)

	if apiKey == "" {
		log.Fatalf("bad environment: can't find GETBLOCK_API_KEY")
	}

	if len(os.Args) >= 2 {
		if nBlocks, err = strconv.ParseInt(os.Args[1], 10, 64); err != nil {
			log.Fatalf("couldn't parse number of blocks to find the account with the most turnover: %+v\n", err)
		}
	}

	getblock := &getBlockApi{
		ApiKey: apiKey,
	}

	blockNumber, err := getblock.getBlockNumber(context.TODO())
	if err != nil {
		log.Fatalf("couldn't get chain head: %+v", err)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	resCh := make(chan map[string]*big.Int, runtime.GOMAXPROCS(0))
	errCh := make(chan error, 1)
	for off := int64(0); off < nBlocks; off++ {
		go processTxsForBlockId(ctx, getblock, blockNumber-off, resCh, errCh)
	}

	var (
		max         = big.NewInt(0)
		maxDeltaAcc string

		accDeltas map[string]*big.Int
	)
	for i := int64(0); i < nBlocks; i++ {
		select {
		case res := <-resCh:
			if accDeltas == nil {
				accDeltas = res
				break
			}

			for acc, value := range res {
				if _, ok := accDeltas[acc]; !ok {
					accDeltas[acc] = value
					continue
				}

				accDeltas[acc].Add(accDeltas[acc], value)
			}
		case err := <-errCh:
			cancel()
			log.Fatalf("error processing tx: %+v\n", err)
		}
	}
	for acc, val := range accDeltas {
		if val.CmpAbs(max) > 0 {
			max = val
			maxDeltaAcc = acc
		}
	}
	maxFloat := new(big.Float).SetInt(max)
	fmt.Printf("Lucky bastard is %s with delta of %s ETH\n", maxDeltaAcc, maxFloat.Quo(maxFloat, WeiInEth).String())
}

func processTxsForBlockId(ctx context.Context, getblock *getBlockApi, number int64, resCh chan map[string]*big.Int, errCh chan error) {
	block, err := getblock.getBlockByNumber(ctx, number, true)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		errCh <- err
		return
	}

	var (
		txValue    = new(big.Int)
		txFee      = new(big.Int)
		txGas      = new(big.Int)
		txGasPrice = new(big.Int)
		res        = make(map[string]*big.Int, len(block.Txs))
	)
	for _, tx := range block.Txs {
		var ok bool

		if _, ok = res[tx.From]; !ok {
			res[tx.From] = big.NewInt(0)
		}

		if txGasPrice, ok = txGasPrice.SetString(tx.GasPrice[2:], 16); !ok {
			errCh <- fmt.Errorf("couldn't parse gas price of '%s'", tx.GasPrice)
			return
		}
		if txGas, ok = txGas.SetString(tx.Gas[2:], 16); !ok {
			errCh <- fmt.Errorf("couldn't parse gas value of '%s'", tx.Gas)
			return
		}
		txFee = txFee.Mul(txGas, txGasPrice)
		res[tx.From].Add(res[tx.From], txFee)

		if txValue, ok = txValue.SetString(tx.Value[2:], 16); !ok {
			errCh <- fmt.Errorf("couldn't parse tx value of '%s'", tx.Value)
			return
		}
		res[tx.From].Add(res[tx.From], txValue)

		//If it's not contract deployment
		if tx.To != "" {
			if _, ok := res[tx.To]; !ok {
				res[tx.To] = big.NewInt(0)
			}
			res[tx.To].Add(res[tx.To], txValue)
		}
	}

	resCh <- res
}
