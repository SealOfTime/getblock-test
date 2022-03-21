package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"sync"
	"syscall"
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

	wg := sync.WaitGroup{}
	resCh := make(chan map[string]*big.Int, runtime.GOMAXPROCS(0))
	errCh := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())

	//Graceful shutdown
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		select {
		case err := <-errCh:
			if err != nil {
				log.Printf("cancelling all the work, because: %+v\n", err)
				cancel()
			}
		case <-sig:
			log.Println("cancelling all the work by user request")
			cancel()
		}
	}()

	for off := int64(0); off < nBlocks; off++ {
		wg.Add(1)
		go func(bn int64) {
			defer wg.Done()
			res, err := getAccountDeltasForBlockByNumber(ctx, getblock, bn)
			if errors.Is(err, context.Canceled) {
				return
			}

			if err != nil {
				errCh <- err
				return
			}
			resCh <- res
		}(blockNumber - off)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		var accDeltas map[string]*big.Int
		for i := int64(0); i < nBlocks; i++ {
			select {
			case res := <-resCh:
				accDeltas = mergeAccountDeltas(accDeltas, res)
			case <-ctx.Done():
				return
			}
		}
		resCh <- accDeltas
	}()

	wg.Wait()
	close(errCh)
	close(resCh)
	if ctx.Err() != nil {
		log.Fatalln("exited prematurely")
	}

	accDeltas := <-resCh
	acc, max := findAccountWithMaxDelta(accDeltas)
	maxFloat := big.NewFloat(0.0).SetInt(max)
	fmt.Printf("Lucky bastard is %s with delta of %s ETH\n", acc, maxFloat.Quo(maxFloat, WeiInEth).String())
}

func mergeAccountDeltas(accDeltas map[string]*big.Int, newDeltas map[string]*big.Int) map[string]*big.Int {
	if accDeltas == nil {
		return newDeltas
	}

	for acc, value := range newDeltas {
		if _, ok := accDeltas[acc]; !ok {
			accDeltas[acc] = value
			continue
		}

		accDeltas[acc].Add(accDeltas[acc], value)
	}
	return accDeltas
}

func findAccountWithMaxDelta(accountDeltas map[string]*big.Int) (account string, delta *big.Int) {
	var (
		max = big.NewInt(0)
	)
	for acc, val := range accountDeltas {
		if val.CmpAbs(max) > 0 {
			max = val
			account = acc
		}
	}
	return account, max
}

func getAccountDeltasForBlockByNumber(ctx context.Context, getblock *getBlockApi, number int64) (map[string]*big.Int, error) {
	block, err := getblock.getBlockByNumber(ctx, number, true)
	if err != nil {
		return nil, fmt.Errorf("error getting block data from getblock: %w", err)
	}

	return getAccountDeltasForBlock(block)
}

func getAccountDeltasForBlock(block EthBlock) (map[string]*big.Int, error) {
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
			return nil, fmt.Errorf("couldn't parse gas price of '%s'", tx.GasPrice)
		}
		if txGas, ok = txGas.SetString(tx.Gas[2:], 16); !ok {
			return nil, fmt.Errorf("couldn't parse gas value of '%s'", tx.Gas)
		}
		if txValue, ok = txValue.SetString(tx.Value[2:], 16); !ok {
			return nil, fmt.Errorf("couldn't parse tx value of '%s'", tx.Value)
		}

		txFee.Mul(txGas, txGasPrice)
		res[tx.From].Add(res[tx.From], txFee)

		res[tx.From].Add(res[tx.From], txValue)

		if tx.IsContractDeployment() {
			continue
		}

		if _, ok := res[tx.To]; !ok {
			res[tx.To] = big.NewInt(0)
		}
		res[tx.To].Add(res[tx.To], txValue)
	}
	return res, nil
}
