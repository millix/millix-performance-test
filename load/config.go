package load

type LoadConfig struct {
	NodeConfigs           []*NodeConfig `json:"nodes"`
	TransactionPerNode    uint          `json:"transactions_per_node"`
	OutputsPerTransaction uint          `json:"outputs_per_transaction"`
	GoroutineCount        uint          `json:"goroutine_count"`
	ReceiverAddressBase   string        `json:"receiver_address_base"`
	ReceiverKeyIdentifier string        `json:"receiver_key_identifier"`
}

type NodeConfig struct {
	IP            string `json:"ip"`
	Port          string `json:"port"`
	ID            string `json:"id"`
	Signature     string `json:"signature"`
	AddressBase   string `json:"address_base"`
	KeyIdentifier string `json:"key_identifier"`
}
