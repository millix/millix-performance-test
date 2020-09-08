package load

import (
	"fmt"
	"github.com/pkg/errors"
	"millix-performance-test/client"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type LoadClient struct {
	nodeIP                string
	nodePort              string
	nodeID                string
	nodeSignature         string
	addressBase           string
	keyIdentifier         string
	receiverAddressBase   string
	receiverKeyIdentifier string
	address               string
	outputsPerTxCount     uint
	goroutineCount        uint
	keyMap                map[string]string
	publicKeyMap          map[string]string
	millixClient          *client.Client
	preparedTransactions  []*client.Transaction
}

func NewLoadClient(millixClient *client.Client, nodeIP, nodePort, nodeID, nodeSignature, addressBase, keyIdentifier, receiverAddressBase, receiverKeyIdentifier string, outputsPerTxCount, goroutineCount uint) *LoadClient {
	return &LoadClient{
		nodeIP:                nodeIP,
		nodePort:              nodePort,
		nodeID:                nodeID,
		nodeSignature:         nodeSignature,
		addressBase:           addressBase,
		keyIdentifier:         keyIdentifier,
		receiverAddressBase:   receiverAddressBase,
		receiverKeyIdentifier: receiverKeyIdentifier,
		address:               fmt.Sprintf("%slal%s", addressBase, keyIdentifier),
		millixClient:          millixClient,
		outputsPerTxCount:     outputsPerTxCount,
		goroutineCount:        goroutineCount,
	}
}

func (lc *LoadClient) Send(base, keyIdentifier string, amount uint) error {
	_, err := lc.millixClient.SendMillix([]*client.ReceiverAmount{&client.ReceiverAmount{Amount: amount, AddressBase: base, KeyIdentifier: keyIdentifier}})
	return err
}

func (lc *LoadClient) Balance() (uint, uint, error) {
	return lc.millixClient.GetBalance(lc.address)
}

func (lc *LoadClient) ObtainKeyMaps() error {
	//if err := lc.millixClient.ObtainAddress(); err != nil {
	//	return "", "", err
	//}
	//
	//lc.keyIdentifier = lc.millixClient.GetKeyIdentifier()
	//lc.address = lc.millixClient.GetAddress()

	keyMap := make(map[string]string)
	publicKeyMap := make(map[string]string)

	privKey, err := lc.millixClient.GetPrivateKey(lc.address)
	if err != nil {
		return err
	}

	keyMap[lc.keyIdentifier] = privKey

	info, err := lc.millixClient.GetAddressInfo(lc.address)
	if err != nil {
		return err
	}

	publicKeyMap[info.AddressBase] = info.AddressAttribute["key_public"]

	lc.keyMap = keyMap
	lc.publicKeyMap = publicKeyMap

	return nil
}

func (lc *LoadClient) PrepareOutputs(totalOutputCount, outputPerTxCount uint) error {
	fmt.Printf("[Load Client] Preparing %d outputs for the load test. %d outputs per transaction\n", totalOutputCount, outputPerTxCount)
	fmt.Printf("[Load Client] Verifying node id.\n")

	if err := lc.millixClient.VerifyNodeID(); err != nil {
		return errors.Wrap(err, "Failed to verify node id")
	}

	fmt.Printf("[Load Client] Fetching available outputs.\n")
	outputs, err := lc.millixClient.GetUnspentTransactionOutputs(lc.keyIdentifier)
	if err != nil {
		return errors.Wrap(err, "Failed to get outputs")
	}

	startOutputCount := uint(len(outputs))

	fmt.Printf("[Load Client] Starting with %d available outputs.\n", startOutputCount)

	if len(outputs) == 0 {
		return errors.New("No outputs")
	}

	sort.Slice(outputs, func(x, y int) bool {
		firstOutput := outputs[x]
		secondOutput := outputs[y]
		return firstOutput.Amount > secondOutput.Amount
	})

	chosenOutput := outputs[0]
	if chosenOutput.Amount < totalOutputCount {
		return errors.New("Insufficient fund in the biggest output")
	}

	rounds := totalOutputCount / outputPerTxCount

	transactions := make([]*client.Transaction, 0)

	for i := uint(1); i <= rounds; i++ {
		fmt.Printf("[Load Client] Round %d. Chose output on transaction %s position %d.\n", i, chosenOutput.TransactionID, chosenOutput.OutputPosition)

		receiverAmounts := make([]*client.ReceiverAmount, 0)
		for j := uint(0); j < outputPerTxCount; j++ {
			receiverAmounts = append(receiverAmounts, &client.ReceiverAmount{Amount: 1, AddressBase: lc.addressBase, KeyIdentifier: lc.keyIdentifier})
		}

		tx, err := lc.millixClient.SendMillixFromOutput(chosenOutput, receiverAmounts)
		if err != nil {
			return errors.Wrap(err, "Failed to send millix")
		}

		if tx == nil {
			panic("No transaction")
		}

		fmt.Printf("[Load Client] Created transaction %s.\n", tx.TransactionID)
		transactions = append(transactions, tx)

		chosenOutput = &client.TransactionOutput{
			Amount:         chosenOutput.Amount - outputPerTxCount,
			TransactionID:  tx.TransactionID,
			ShardID:        tx.ShardID,
			AddressVersion: chosenOutput.AddressVersion,
			AddressBase:    chosenOutput.AddressBase,
			Address:        chosenOutput.Address,
		}
	}

	fmt.Printf("[Load Client] Done preparing. Sleeping for 60 seconds\n")
	time.Sleep(time.Second * 60)

	outputs, err = lc.millixClient.GetUnspentTransactionOutputs(lc.keyIdentifier)
	if err != nil {
		return errors.Wrap(err, "Failed to get outputs")
	}

	lc.preparedTransactions = transactions

	return nil
}

func (lc *LoadClient) prepareTransactions() []*client.UnsignedTransaction {
	unsignedTransactions := make([]*client.UnsignedTransaction, 0)

	for _, transaction := range lc.preparedTransactions {
		// Starting from 1 to skip the change output
		for i := uint(1); i <= lc.outputsPerTxCount; i++ {
			input := &client.TransactionInput{
				AddressBase:          lc.keyIdentifier,
				AddressKeyIdentifier: lc.keyIdentifier,
				AddressVersion:       "lal",
				OutputPosition:       i,
				OutputShardID:        transaction.ShardID,
				//OutputTransactionDate: transaction.TransactionDate,
				OutputTransactionID: transaction.TransactionID,
			}

			output := &client.TransactionOutput{
				AddressBase:          lc.receiverAddressBase,
				AddressVersion:       "lal",
				AddressKeyIdentifier: lc.receiverKeyIdentifier,
				Amount:               1,
			}

			unsignedTx := &client.UnsignedTransaction{
				TransactionVersion: "la0l",
				InputList:          []*client.TransactionInput{input},
				OutputList:         []*client.TransactionOutput{output},
			}

			unsignedTransactions = append(unsignedTransactions, unsignedTx)
		}
	}

	fmt.Printf("[Load Client] Prepared %d unsigned transactions\n\n", len(unsignedTransactions))

	return unsignedTransactions
}

func (lc *LoadClient) SendTransactions() error {
	unsignedTransactions := lc.prepareTransactions()

	unsignedTxChannel := make(chan *client.UnsignedTransaction, lc.goroutineCount)

	go func() {
		for _, unsignedTransaction := range unsignedTransactions {
			unsignedTxChannel <- unsignedTransaction
		}

		close(unsignedTxChannel)
	}()

	wg := sync.WaitGroup{}
	wg.Add(int(lc.goroutineCount))
	var totalCount int32

	startTime := time.Now()
	fmt.Printf("[Load Client] Starting. Time: %v\n", startTime)

	for i := uint(0); i < lc.goroutineCount; i++ {
		go func(id uint) {
			fmt.Printf("[Load Client] Starting client %d.\n", id)

			defer func() {
				fmt.Printf("[Load Client] Done client %d.\n", id)
				wg.Done()
			}()

			count := 0

			millixClient := client.NewClient(lc.nodeIP, lc.nodePort, lc.nodeID, lc.nodeSignature, lc.addressBase, lc.keyIdentifier)
			if err := millixClient.ObtainAddress(); err != nil {
				fmt.Printf("[Load Client] Error: %s\n", err)
				return
			}

			for unsignedTransaction := range unsignedTxChannel {
				for j := 0; j < 5; j++ {
					tx, err := millixClient.SignTransaction(unsignedTransaction, lc.keyMap, lc.publicKeyMap)
					if err != nil {
						fmt.Printf("[Load Client] ID: %d. Attempt %d. Error: %s\n", id, j, err)
						if j == 5 {
							fmt.Printf("[Load Client] ID: %d. Aborting", id)
							return
						}
					} else {
						if err := millixClient.SubmitTransaction(tx); err != nil {
							fmt.Printf("[Load Client] Error: %s\n", err)
							return
						}

						if count%100 == 0 {
							fmt.Printf("[Load Client] ID: %d. Transaction %d. Hash: %s.\n", id, count, tx.TransactionID)
						}
						count++
						atomic.AddInt32(&totalCount, 1)
						break
					}
				}
			}
		}(i)
	}

	wg.Wait()

	endTime := time.Now()
	fmt.Printf("[Load Client] Done. Address: %s. %d clients. %d transactions sent. Time: %v\n", lc.address, lc.goroutineCount, totalCount, endTime)

	diff := endTime.Sub(startTime)
	fmt.Printf("[Load Client] Total duration: %v. Seconds: %f. Tx/s: %f\n", diff, diff.Seconds(), float64(totalCount)/diff.Seconds())

	return nil
}
