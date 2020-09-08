package client

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"
	"time"
)

type Client struct {
	ip            string
	port          string
	nodeID        string
	nodeSignature string
	addressBase   string
	keyIdentifier string
	address       string
	httpClient    http.Client
}

func NewClient(ip, port, nodeID, nodeSignature, addressBase, keyIdentifier string) *Client {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := http.Client{
		Transport: tr,
	}

	return &Client{
		ip:            ip,
		port:          port,
		nodeID:        nodeID,
		nodeSignature: nodeSignature,
		addressBase:   addressBase,
		keyIdentifier: keyIdentifier,
		address:       fmt.Sprintf("%slal%s", addressBase, keyIdentifier),
		httpClient:    client,
	}
}

type AddressInfo struct {
	Address              string            `json:"address"`
	AddressBase          string            `json:"address_base"`
	AddressVersion       string            `json:"address_version"`
	AddressKeyIdentifier string            `json:"address_key_identifier"`
	WalletID             string            `json:"wallet_id"`
	AddressAttribute     map[string]string `json:"address_attribute"`
}

type Transaction struct {
	TransactionID   string                   `json:"transaction_id"`
	Inputs          []*TransactionInput      `json:"transaction_input_list"`
	Outputs         []*NewTransactionOutput  `json:"transaction_output_list"`
	SignatureList   []map[string]interface{} `json:"transaction_signature_list"`
	ParentList      []string                 `json:"transaction_parent_list"`
	PayloadHash     string                   `json:"payload_hash"`
	TransactionDate string                   `json:"transaction_date"`
	ShardID         string                   `json:"shard_id"`
	Version         string                   `json:"version"`
	NodeIDOrigin    string                   `json:"node_id_origin"`
}

type UnsignedTransaction struct {
	TransactionVersion string               `json:"transaction_version"`
	OutputList         []*TransactionOutput `json:"transaction_output_list"`
	InputList          []*TransactionInput  `json:"transaction_input_list"`
}

type NewTransactionOutput struct {
	OutputPosition       uint   `json:"output_position"`
	AddressBase          string `json:"address_base"`
	AddressKeyIdentifier string `json:"address_key_identifier"`
	Amount               uint   `json:"amount"`
	AddressVersion       string `json:"address_version"`
}

type TransactionOutput struct {
	TransactionID        string `json:"transaction_id"`
	ShardID              string `json:"shard_id"`
	OutputPosition       uint   `json:"output_position"`
	Address              string `json:"address"`
	AddressBase          string `json:"address_base"`
	AddressKeyIdentifier string `json:"address_key_identifier"`
	Amount               uint   `json:"amount"`
	TransactionDate      uint   `json:"transaction_date"`
	AddressVersion       string `json:"address_version"`
}

type TransactionInput struct {
	AddressBase           string `json:"address_base"`
	AddressKeyIdentifier  string `json:"address_key_identifier"`
	AddressVersion        string `json:"address_version"`
	OutputPosition        uint   `json:"output_position"`
	OutputTransactionDate uint   `json:"output_transaction_date"`
	OutputTransactionID   string `json:"output_transaction_id"`
	OutputShardID         string `json:"output_shard_id"`
	InputPosition         uint   `json:"input_position"`
}

type ReceiverAmount struct {
	KeyIdentifier string
	AddressBase   string
	Amount        uint
}

func (c *Client) VerifyNodeID() error {
	url := fmt.Sprintf("%s/%s", c.getBaseUrl(), "ZFAYRM8LRtmfYp4Y")

	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	q := request.URL.Query()

	request.URL.RawQuery = q.Encode()

	resp, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	respContent, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	idResp := make(map[string]string)

	if err = json.Unmarshal(respContent, &idResp); err != nil {
		return err
	}

	if c.nodeID != idResp["node_id"] {
		return errors.New("Invalid node id")
	}

	return nil
}

func (c *Client) ObtainAddress() error {
	info, err := c.GenerateNewAddress()
	if err != nil {
		return err
	}

	c.keyIdentifier = info.AddressKeyIdentifier
	c.address = fmt.Sprintf("%slal%s", info.AddressKeyIdentifier, info.AddressKeyIdentifier)

	return nil
}

func (c *Client) GetAddress() string {
	return c.address
}

func (c *Client) GetKeyIdentifier() string {
	return c.keyIdentifier
}

func (c *Client) SendMillixFromOutput(output *TransactionOutput, receiverAmounts []*ReceiverAmount) (*Transaction, error) {
	var neededAmount uint
	for _, receiverAmount := range receiverAmounts {
		neededAmount += receiverAmount.Amount
	}

	input := &TransactionInput{
		AddressBase:           c.keyIdentifier,
		AddressKeyIdentifier:  c.keyIdentifier,
		AddressVersion:        "lal",
		OutputPosition:        output.OutputPosition,
		OutputShardID:         output.ShardID,
		OutputTransactionDate: output.TransactionDate,
		OutputTransactionID:   output.TransactionID,
	}

	newOutputs := make([]*TransactionOutput, 0)

	change := output.Amount - neededAmount
	if change > 0 {
		changeOutput := &TransactionOutput{
			AddressKeyIdentifier: c.keyIdentifier,
			AddressBase:          c.keyIdentifier,
			Amount:               change,
			AddressVersion:       "lal",
		}

		newOutputs = append(newOutputs, changeOutput)
	}

	for _, receiverAmount := range receiverAmounts {
		output := &TransactionOutput{
			AddressBase:          receiverAmount.AddressBase,
			AddressVersion:       "lal",
			AddressKeyIdentifier: receiverAmount.KeyIdentifier,
			Amount:               receiverAmount.Amount,
		}

		newOutputs = append(newOutputs, output)
	}

	unsignedTx := &UnsignedTransaction{
		TransactionVersion: "la0l",
		OutputList:         newOutputs,
		InputList:          []*TransactionInput{input},
	}

	keyMap := make(map[string]string)
	publicKeyMap := make(map[string]string)

	privKey, err := c.GetPrivateKey(c.address)
	if err != nil {
		return nil, err
	}

	keyMap[c.keyIdentifier] = privKey

	info, err := c.GetAddressInfo(c.address)
	if err != nil {
		return nil, err
	}

	publicKeyMap[info.AddressBase] = info.AddressAttribute["key_public"]

	tx, err := c.SignTransaction(unsignedTx, keyMap, publicKeyMap)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to sign transaction")
	}

	if err := c.SubmitTransaction(tx); err != nil {
		return nil, nil
	}

	return tx, nil
}

func (c *Client) SendMillix(receiverAmounts []*ReceiverAmount) (*Transaction, error) {
	var neededAmount uint
	for _, receiverAmount := range receiverAmounts {
		neededAmount += receiverAmount.Amount
	}

	for i := 1; i <= 11; i++ {
		if i == 11 {
			return nil, errors.New("Failed to stabilize balance")
		}
		stable, unstable, err := c.GetBalance(c.address)
		if err != nil {
			return nil, err
		}

		fmt.Printf("[Client] Stable: %d. Unstable: %d\n", stable, unstable)

		if unstable > 0 {
			fmt.Printf("[Client] Sleeping. Stable %d. Unstable %d\n", stable, unstable)
			time.Sleep(time.Second * time.Duration(i))
		} else {
			break
		}
	}

	outputs, err := c.GetUnspentTransactionOutputs(c.keyIdentifier)
	if err != nil {
		return nil, err
	}

	fmt.Printf("[Client] Total %d outputs\n", len(outputs))

	sort.Slice(outputs, func(x, y int) bool {
		firstOutput := outputs[x]
		secondOutput := outputs[y]
		return firstOutput.Amount > secondOutput.Amount
	})

	if len(outputs) == 0 {
		return nil, errors.New("No outputs to consume")
	}

	var chosenAmount uint
	chosenOutputs := make([]*TransactionOutput, 0)

	addresses := make(map[string]string)

	for _, output := range outputs {
		chosenAmount += output.Amount
		addresses[output.Address] = output.AddressKeyIdentifier
		chosenOutputs = append(chosenOutputs, output)

		fmt.Printf("[Client] Chose output from transaction %s and position %d.\n", output.TransactionID, output.OutputPosition)

		if chosenAmount >= neededAmount {
			break
		}
	}

	keyMap := make(map[string]string)
	publicKeyMap := make(map[string]string)

	for address, keyIdentifier := range addresses {
		privKey, err := c.GetPrivateKey(address)
		if err != nil {
			return nil, err
		}

		keyMap[keyIdentifier] = privKey

		info, err := c.GetAddressInfo(address)
		if err != nil {
			return nil, err
		}

		publicKeyMap[info.AddressBase] = info.AddressAttribute["key_public"]
	}

	fmt.Printf("[Client] Chose %d outputs. Total chosen amount: %d. Needed: %d\n", len(chosenOutputs), chosenAmount, neededAmount)

	inputs := make([]*TransactionInput, 0)

	for _, output := range chosenOutputs {
		input := &TransactionInput{
			AddressBase:           output.AddressKeyIdentifier,
			AddressKeyIdentifier:  output.AddressKeyIdentifier,
			AddressVersion:        "lal",
			OutputPosition:        output.OutputPosition,
			OutputShardID:         output.ShardID,
			OutputTransactionDate: output.TransactionDate,
			OutputTransactionID:   output.TransactionID,
		}

		inputs = append(inputs, input)
	}

	newOutputs := make([]*TransactionOutput, 0)

	for _, receiverAmount := range receiverAmounts {
		output := &TransactionOutput{
			AddressBase:          receiverAmount.AddressBase,
			AddressVersion:       "lal",
			AddressKeyIdentifier: receiverAmount.KeyIdentifier,
			Amount:               receiverAmount.Amount,
		}

		newOutputs = append(newOutputs, output)
	}

	change := chosenAmount - neededAmount
	if change > 0 {
		changeOutput := &TransactionOutput{
			AddressKeyIdentifier: c.keyIdentifier,
			AddressBase:          c.keyIdentifier,
			Amount:               change,
			AddressVersion:       "lal",
		}

		newOutputs = append(newOutputs, changeOutput)
	}

	// Sanity check
	var totalInput, totalOutput uint

	for _, o := range chosenOutputs {
		totalInput += o.Amount
	}
	for _, o := range newOutputs {
		totalOutput += o.Amount
	}

	fmt.Printf("[Client] Total input: %d. Total output: %d\n", totalInput, totalInput)

	if totalInput != totalOutput {
		panic(fmt.Sprintf("Total input %d vs Total output %d\n", totalInput, totalOutput))
	}

	unsignedTx := &UnsignedTransaction{
		TransactionVersion: "la0l",
		OutputList:         newOutputs,
		InputList:          inputs,
	}

	tx, err := c.SignTransaction(unsignedTx, keyMap, publicKeyMap)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to sign transaction")
	}

	if err := c.SubmitTransaction(tx); err != nil {
		return nil, errors.Wrap(err, "Failed to submit transaction")
	}

	return tx, nil
}

func (c *Client) GetUnspentTransactionOutputs(addressKeyIdentifier string) ([]*TransactionOutput, error) {
	url := fmt.Sprintf("%s/%s", c.getBaseUrl(), "FDLyQ5uo5t7jltiQ")

	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	q := request.URL.Query()

	q.Add("p3", addressKeyIdentifier)
	q.Add("p7", "1")
	q.Add("p10", "0")
	q.Add("p14", "10000000")

	request.URL.RawQuery = q.Encode()

	resp, err := c.httpClient.Do(request)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	respContent, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	outputs := make([]*TransactionOutput, 0)
	if err := json.Unmarshal(respContent, &outputs); err != nil {
		return nil, err
	}

	return outputs, nil
}

type privateKeyResponse struct {
	Key string `json:"private_key_hex"`
}

func (c *Client) GetPrivateKey(address string) (string, error) {
	url := fmt.Sprintf("%s/%s", c.getBaseUrl(), "PKUv2JfV87KpEZwE")

	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	q := request.URL.Query()

	q.Add("p0", address)

	request.URL.RawQuery = q.Encode()

	resp, err := c.httpClient.Do(request)
	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	respContent, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var keyResp *privateKeyResponse
	if err := json.Unmarshal(respContent, &keyResp); err != nil {
		return "", err
	}

	return keyResp.Key, nil
}

type signRequest struct {
	UnsignedTx   *UnsignedTransaction `json:"p0"`
	KeyMap       map[string]string    `json:"p1"`
	PublicKeyMap map[string]string    `json:"p2"`
}

func (c *Client) SignTransaction(unsignedTx *UnsignedTransaction, keyMap map[string]string, publicKeyMap map[string]string) (*Transaction, error) {
	url := fmt.Sprintf("%s/%s", c.getBaseUrl(), "RVBqKlGdk9aEhi5J")

	r := &signRequest{
		UnsignedTx:   unsignedTx,
		KeyMap:       keyMap,
		PublicKeyMap: publicKeyMap,
	}

	signRequestJson, err := json.Marshal(r)
	if err != nil {
		panic(fmt.Errorf("Failed to marshal to json: %s", err))
	}

	request, err := http.NewRequest("POST", url, bytes.NewBuffer(signRequestJson))
	if err != nil {
		return nil, err
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Content-Length", strconv.Itoa(len(signRequestJson)))

	resp, err := c.httpClient.Do(request)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	respContent, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var transaction *Transaction
	if err := json.Unmarshal(respContent, &transaction); err != nil {
		return nil, err
	}

	if transaction.TransactionID == "" {
		resp := make(map[string]string)

		if err := json.Unmarshal(respContent, &resp); err != nil {
			return nil, err
		}

		if resp["status"] == "fail" {
			return nil, errors.New("Status fail")
		}
	}

	return transaction, nil
}

type submitTransactionResponse struct {
	Status string `json:"status"`
}

type submitTransactionRequest struct {
	Transaction *Transaction `json:"p0"`
}

func (c *Client) SubmitTransaction(tx *Transaction) error {
	url := fmt.Sprintf("%s/%s", c.getBaseUrl(), "VnJIBrrM0KY3uQ9X")
	r := &submitTransactionRequest{
		Transaction: tx,
	}

	submitRequestJson, err := json.Marshal(r)
	if err != nil {
		panic(fmt.Errorf("Failed to marshal: %s", err))
	}

	request, err := http.NewRequest("POST", url, bytes.NewBuffer(submitRequestJson))
	if err != nil {
		return err
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Content-Length", strconv.Itoa(len(submitRequestJson)))

	resp, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	respContent, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var submitResp *submitTransactionResponse
	json.Unmarshal(respContent, &submitResp)

	if submitResp.Status != "success" {
		return fmt.Errorf("Got status: %s", submitResp.Status)
	}

	return nil
}

func (c *Client) GetAddressInfo(address string) (*AddressInfo, error) {
	url := fmt.Sprintf("%s/%s", c.getBaseUrl(), "ywTmt3C0nwk5k4c7")

	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	q := request.URL.Query()

	q.Add("p0", address)

	request.URL.RawQuery = q.Encode()

	resp, err := c.httpClient.Do(request)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	respContent, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var info *AddressInfo
	if err := json.Unmarshal(respContent, &info); err != nil {
		return nil, err
	}

	return info, nil
}

type balanceInfo struct {
	Stable   uint `json:"stable"`
	Unstable uint `json:"unstable"`
}

func (c *Client) GetBalance(address string) (uint, uint, error) {
	url := fmt.Sprintf("%s/%s", c.getBaseUrl(), "zLsiAkocn90e3K6R")

	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, 0, err
	}

	q := request.URL.Query()

	q.Add("p0", address)

	request.URL.RawQuery = q.Encode()

	resp, err := c.httpClient.Do(request)
	if err != nil {
		return 0, 0, err
	}

	defer resp.Body.Close()

	respContent, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, 0, err
	}

	var info *balanceInfo
	if err := json.Unmarshal(respContent, &info); err != nil {
		return 0, 0, err
	}

	return info.Stable, info.Unstable, nil
}

func (c *Client) GenerateNewAddress() (*AddressInfo, error) {
	url := fmt.Sprintf("%s/%s", c.getBaseUrl(), "Lb2fuhVMDQm1DrLL")

	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	q := request.URL.Query()

	request.URL.RawQuery = q.Encode()

	resp, err := c.httpClient.Do(request)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	respContent, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var info *AddressInfo
	if err = json.Unmarshal(respContent, &info); err != nil {
		return nil, err
	}

	return info, nil
}

func (c *Client) getBaseUrl() string {
	return fmt.Sprintf("https://%s:%s/api/%s/%s", c.ip, c.port, c.nodeID, c.nodeSignature)
}
