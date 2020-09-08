package load

import (
	"fmt"
	"github.com/pkg/errors"
	"millix-performance-test/client"
	"time"
)

type Orchestrator struct {
	funderClient              *client.Client
	funderAddress             string
	millixClients             map[string]*client.Client
	loadClients               map[string]*LoadClient
	nodeConfigs               []*NodeConfig
	transactionPerNode        uint
	outputPerTransactionCount uint
	startingBalances          map[string]uint
}

func NewOrchestrator(config *LoadConfig) *Orchestrator {
	millixClients := make(map[string]*client.Client)
	loadClients := make(map[string]*LoadClient)
	var funderClient *client.Client
	var funderAddress string

	for i, nodeConfig := range config.NodeConfigs {
		nodeAddress := fmt.Sprintf("%slal%s", nodeConfig.AddressBase, nodeConfig.KeyIdentifier)
		millixClient := client.NewClient(nodeConfig.IP, nodeConfig.Port, nodeConfig.ID, nodeConfig.Signature, nodeConfig.AddressBase, nodeConfig.KeyIdentifier)
		millixClients[nodeAddress] = millixClient

		if i == 0 {
			funderClient = millixClient
			funderAddress = nodeConfig.KeyIdentifier
		}

		loadClient := NewLoadClient(millixClient, nodeConfig.IP, nodeConfig.Port, nodeConfig.ID, nodeConfig.Signature, nodeConfig.AddressBase, nodeConfig.KeyIdentifier, config.ReceiverAddressBase, config.ReceiverKeyIdentifier, config.OutputsPerTransaction, config.GoroutineCount)
		loadClients[nodeAddress] = loadClient
	}

	return &Orchestrator{
		funderClient:              funderClient,
		funderAddress:             funderAddress,
		millixClients:             millixClients,
		loadClients:               loadClients,
		nodeConfigs:               config.NodeConfigs,
		transactionPerNode:        config.TransactionPerNode,
		outputPerTransactionCount: config.OutputsPerTransaction,
		startingBalances:          make(map[string]uint),
	}
}

func (o *Orchestrator) Load() (*Result, error) {
	totalTransactionCount := uint(len(o.nodeConfigs)) * o.transactionPerNode
	fmt.Printf("[Orchestrator] Starting load test. %d nodes. %d total transactions.\n", len(o.nodeConfigs), totalTransactionCount)

	err := o.ensureFunds()
	if err != nil {
		return nil, errors.Wrap(err, "Failed to prepare initial funds")
	}

	err = o.prepareOutputs()
	if err != nil {
		return nil, errors.Wrap(err, "Failed to prepare transaction outputs")
	}

	startTime := time.Now()

	err = o.sendTransactions()
	if err != nil {
		return nil, errors.Wrap(err, "Failed to perform load test")
	}

	endTime := time.Now()
	achievedTps := float64(totalTransactionCount) / endTime.Sub(startTime).Seconds()

	res := &Result{
		StartTime:         &startTime,
		EndTime:           &endTime,
		NodeCount:         uint(len(o.nodeConfigs)),
		TotalTransactions: totalTransactionCount,
		AchievedTps:       achievedTps,
	}

	return res, nil
}

// Ensures that all the nodes have enough funds to perform the required load test
// The first node is assumed to have enough funds (funded in genesis)
func (o *Orchestrator) ensureFunds() error {
	fmt.Printf("[Orchestrator][Step 1] Ensuring that all of the nodes have sufficient funds.\n")

	nodeSender := o.funderClient

	receiverAmounts := make([]*client.ReceiverAmount, 0)

	// Skipping the first node
	for i := 1; i < len(o.nodeConfigs); i++ {
		nodeConfig := o.nodeConfigs[i]
		receiverAmounts = append(receiverAmounts, &client.ReceiverAmount{AddressBase: nodeConfig.AddressBase, KeyIdentifier: nodeConfig.KeyIdentifier, Amount: o.transactionPerNode})
	}

	tx, err := nodeSender.SendMillix(receiverAmounts)
	if err != nil {
		return errors.Wrap(err, "Failed to send initial amounts to nodes")
	}

	fmt.Printf("[Orchestrator][Step 1] Nodes funding transaction: %s.\n", tx.TransactionID)
	fmt.Printf("[Orchestrator][Step 1] Waiting for nodes to have stable balance. Sleeping 15 seconds\n")
	time.Sleep(time.Second * 15)

	allStable := false

Outer:
	for i := 0; i < 12; i++ {
		if allStable {
			break
		}
		fmt.Printf("[Orchestrator][Step 1] Sleeping for %d seconds.\n", i*2)
		time.Sleep(time.Second * time.Duration(i*2))

		for address, millixClient := range o.millixClients {
			stable, unstable, err := millixClient.GetBalance(address)
			if err != nil {
				fmt.Printf("[Orchestrator][Step 1] ERROR. Failed to get balance: %s.\n", err)
				continue Outer
			}

			if unstable > 0 {
				fmt.Printf("[Orchestrator][Step 1] Address %s still has %d unstable.\n", address, unstable)
				continue Outer

			} else if stable == 0 {
				fmt.Printf("[Orchestrator][Step 1] Address %s stil has 0 stable.\n", address)
				continue Outer
			} else {
				fmt.Printf("[Orchestrator][Step 1] Address %s has %d stable.\n", address, stable)
				o.startingBalances[address] = stable
			}
		}

		allStable = true
	}

	if !allStable {
		fmt.Printf("[Orchestrator][Step 1] FAILED TO STABILISE.\n")
		return errors.New("Failed to stabilise")
	}

	fmt.Printf("[Orchestrator][Step 1] Balances are stable.\n")
	fmt.Printf("[Orchestrator][Step 1] All nodes have sufficient funds.\n")

	return nil
}

type prepareOutputsRes struct {
	Address string
	Err     error
}

// Prepares outputs by instructing all individual load clients to prepare outputs
func (o *Orchestrator) prepareOutputs() error {
	fmt.Printf("[Orchestrator][Step 2] Preparing transaction outputs.\n")

	resCh := make(chan *prepareOutputsRes, len(o.loadClients))
	for address, loadClient := range o.loadClients {
		go func(address string, loadClient *LoadClient) {
			err := loadClient.PrepareOutputs(o.transactionPerNode, o.outputPerTransactionCount)
			resCh <- &prepareOutputsRes{
				Err:     err,
				Address: address,
			}
		}(address, loadClient)
	}

	fmt.Printf("[Orchestrator][Step 2] Waiting for prepare outputs results.\n")

	for i := 0; i < len(o.loadClients); i++ {
		res := <-resCh
		if res.Err != nil {
			return errors.Wrap(res.Err, fmt.Sprintf("Failed to prepare output on node %s.", res.Address))
		}

		fmt.Printf("[Orchestrator][Step 2] Node %s successfully prepared outputs.\n", res.Address)
	}

	fmt.Printf("[Orchestrator][Step 2] Outputs successfully prepared.\n")

	return nil
}

type sendTransactionsRes struct {
	Address string
	Err     error
}

// Instructs all the load clients to send transactions
func (o *Orchestrator) sendTransactions() error {
	fmt.Printf("[Orchestrator][Step 3] Sending transactions.\n")
	resCh := make(chan *sendTransactionsRes, len(o.loadClients))

	for address, loadClient := range o.loadClients {
		go func(address string, loadClient *LoadClient) {
			if err := loadClient.ObtainKeyMaps(); err != nil {
				resCh <- &sendTransactionsRes{
					Address: address,
					Err:     err,
				}
				return
			}

			err := loadClient.SendTransactions()
			resCh <- &sendTransactionsRes{
				Address: address,
				Err:     err,
			}
		}(address, loadClient)
	}

	fmt.Printf("[Orchestrator][Step 3] Waiting for send transactions results.\n")

	for i := 0; i < len(o.loadClients); i++ {
		res := <-resCh
		if res.Err != nil {
			return errors.Wrap(res.Err, fmt.Sprintf("Failed to send transactions on node %s", res.Address))
		}

		fmt.Printf("[Orchestrator][Step 3] Node %s successfully sent transactions.\n", res.Address)
	}

	fmt.Printf("[Orchestrator] All transactions successfully sent.\n")

	return nil
}
