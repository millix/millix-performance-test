package client

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"io/ioutil"
	"net/http"
	"os"
)

type Client struct {
	httpClient    http.Client
	keyIdentifier string
}

func NewClient(keyIdentifier string) *Client {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := http.Client{
		Transport: tr,
	}

	return &Client{
		httpClient:    client,
		keyIdentifier: keyIdentifier,
	}
}

var (
	NODE_ID        string
	NODE_SIGNATURE string
)

func init() {
	NODE_ID = os.Getenv("NODE_ID")
	if NODE_ID == "" {
		panic("Missing NODE_ID environment variable")
	}

	NODE_SIGNATURE = os.Getenv("NODE_SIGNATURE")
	if NODE_SIGNATURE == "" {
		panic("Missing NODE_SIGNATURE environment variable")
	}
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

func (c *Client) SendMillix(amount uint, receiver string) (string, error) {
	outputs, err := c.GetUnspentTransactionOutputs(c.keyIdentifier)
	if err != nil {
		return "", err
	}

	var chosenAmount uint
	chosenOutputs := make([]*TransactionOutput, 0)

	addresses := make(map[string]string)

	for _, output := range outputs {
		chosenAmount += output.Amount
		addresses[output.Address] = output.AddressKeyIdentifier
		chosenOutputs = append(chosenOutputs, output)

		if chosenAmount >= amount {
			break
		}
	}

	keyMap := make(map[string]string)

	for address, keyIdentifier := range addresses {
		privKey, err := c.GetPrivateKey(address)
		if err != nil {
			return "", err
		}

		keyMap[keyIdentifier] = privKey
	}

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

	changeOutput := &TransactionOutput{
		AddressKeyIdentifier: c.keyIdentifier,
		AddressBase:          c.keyIdentifier,
		Amount:               chosenAmount - amount,
		AddressVersion:       "lal",
	}

	output := &TransactionOutput{
		AddressBase:          receiver,
		AddressVersion:       "lal",
		AddressKeyIdentifier: receiver,
		Amount:               amount,
	}

	newOutputs := []*TransactionOutput{output, changeOutput}

	unsignedTx := &UnsignedTransaction{
		TransactionVersion: "la0l",
		OutputList:         newOutputs,
		InputList:          inputs,
	}

	tx, err := c.SignTransaction(unsignedTx, keyMap)
	if err != nil {
		return "", errors.Wrap(err, "Failed to sign client")
	}

	if err := c.SubmitTransaction(tx); err != nil {
		return "", nil
	}

	return tx.TransactionID, nil
}

func (c *Client) GetUnspentTransactionOutputs(addressKeyIdentifier string) ([]*TransactionOutput, error) {
	url := fmt.Sprintf("%s/%s", getBaseUrl(), "FDLyQ5uo5t7jltiQ")

	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	q := request.URL.Query()

	q.Add("p3", addressKeyIdentifier)
	q.Add("p10", "0")
	q.Add("p7", "1")

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
	url := fmt.Sprintf("%s/%s", getBaseUrl(), "PKUv2JfV87KpEZwE")

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

func (c *Client) SignTransaction(unsignedTx *UnsignedTransaction, keyMap map[string]string) (*Transaction, error) {
	url := fmt.Sprintf("%s/%s", getBaseUrl(), "RVBqKlGdk9aEhi5J")

	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	unsignedTransactionJson, err := json.Marshal(unsignedTx)
	if err != nil {
		return nil, err
	}

	keyMapJson, err := json.Marshal(keyMap)
	if err != nil {
		return nil, err
	}

	q := request.URL.Query()

	q.Add("p0", string(unsignedTransactionJson))
	q.Add("p1", string(keyMapJson))

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

	var transaction *Transaction
	if err := json.Unmarshal(respContent, &transaction); err != nil {
		return nil, err
	}

	return transaction, nil
}

type submitTransactionResponse struct {
	Status string `json:"status"`
}

func (c *Client) SubmitTransaction(tx *Transaction) error {
	url := fmt.Sprintf("%s/%s", getBaseUrl(), "VnJIBrrM0KY3uQ9X")

	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	transactionJson, err := json.Marshal(tx)
	if err != nil {
		return err
	}

	q := request.URL.Query()

	q.Add("p0", string(transactionJson))

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

	var submitResp *submitTransactionResponse
	json.Unmarshal(respContent, &submitResp)

	if submitResp.Status != "success" {
		return fmt.Errorf("Got status: %s", submitResp.Status)
	}

	return nil
}

func getBaseUrl() string {
	return fmt.Sprintf("https://localhost:5500/api/%s/%s", NODE_ID, NODE_SIGNATURE)
}
