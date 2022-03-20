package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
)

type EthBlock struct {
	Number  string `json:"number"`
	Miner   string `json:"miner"`
	GasUsed string `json:"gasUsed"`
	//baseFeePerGas in wei
	BaseFeePerGas string  `json:"baseFeePerGas"`
	Txs           []EthTx `json:"transactions"`
}

type EthTx struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Gas      string `json:"gas"`      // gasUnits
	GasPrice string `json:"gasPrice"` // feePerGas = base fee + tip
	Value    string `json:"value"`    // gwei
}

type getBlockApi struct {
	ApiKey string
}

func (a *getBlockApi) getBlockNumber(ctx context.Context) (int64, error) {
	type blockNumberResp struct {
		BlockId string `json:"result"`
	}
	resp, err := callEthNode[blockNumberResp](ctx, a.ApiKey, "eth_blockNumber")
	if err != nil {
		return 0, fmt.Errorf("couldn't get last block number: %w", err)
	}

	numberInt, err := strconv.ParseInt(resp.BlockId[2:], 16, 64)
	if err != nil {
		return 0, fmt.Errorf("block number is not hex int: %w", err)
	}

	return numberInt, nil
}

func (a *getBlockApi) getBlockByNumber(ctx context.Context, number int64, withTxs bool) (EthBlock, error) {
	type getBlockByNumberResp struct {
		Block EthBlock `json:"result"`
	}
	numberHex := fmt.Sprintf("0x%x", number)
	ledgerHead, err := callEthNode[getBlockByNumberResp](ctx, a.ApiKey, "eth_getBlockByNumber", numberHex, withTxs)
	if err != nil {
		return EthBlock{}, fmt.Errorf("couldn't get last block: %w", err)
	}
	return ledgerHead.Block, nil
}

type jsonRpcBody struct {
	Version string `json:"jsonrpc"`
	Method  string `json:"method"`
	Id      string `json:"id"`
	Params  []any  `json:"params"`
}

func callEthNode[Response any](ctx context.Context, apiKey string, method string, params ...any) (resp Response, err error) {
	body, err := json.Marshal(jsonRpcBody{
		Version: "2.0",
		Id:      "getblock.io",
		Method:  method,
		Params:  params,
	})
	if err != nil {
		return resp, fmt.Errorf("couldn't marshal call body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "https://eth.getblock.io/mainnet/", bytes.NewBuffer(body))
	if err != nil {
		return resp, fmt.Errorf("couldn't create request: %w", err)
	}
	req.Header.Add("x-api-key", apiKey)
	req.Header.Add("Content-Type", "application/json")

	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return resp, fmt.Errorf("couldn't call method %s on eth node: %w", method, err)
	}

	respBytes, err := ioutil.ReadAll(httpResp.Body)
	if err != nil {
		return resp, fmt.Errorf("couldn't read response: %w", err)
	}
	if err = json.Unmarshal(respBytes, &resp); err != nil {
		log.Printf("full response body: %s\n", respBytes)
		return resp, fmt.Errorf("couldn't unmarshal response: %w", err)
	}

	return resp, nil
}
