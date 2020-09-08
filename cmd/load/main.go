package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"millix-performance-test/load"
	"os"
)

func main() {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		panic("Missing CONFIG_PATH")
	}

	resPath := os.Getenv("RESULT_PATH")
	if resPath == "" {
		resPath = "result.json"
	}

	configFile, err := os.Open(configPath)
	if err != nil {
		panic(fmt.Sprintf("Failed to open config: %s", err))
	}

	configContent, err := ioutil.ReadAll(configFile)
	if err != nil {
		panic(fmt.Sprintf("Failed to read config: %s", err))
	}

	var config *load.LoadConfig
	err = json.Unmarshal(configContent, &config)
	if err != nil {
		panic(fmt.Sprintf("Failed to unmarshal config: %s", err))
	}

	orchestrator := load.NewOrchestrator(config)
	loadRes, err := orchestrator.Load()
	if err != nil {
		panic(fmt.Sprintf("Orchestrator finished with error: %s\n", err))
	}

	fmt.Printf("Writing result to %s.\n", resPath)

	resFile, err := os.Create(resPath)
	if err != nil {
		panic(fmt.Sprintf("Failed to create result file"))
	}

	resultJson, err := json.MarshalIndent(loadRes, "", "\t")
	if err != nil {
		panic(fmt.Sprintf("Failed to marshal result json: %s", err))
	}

	if _, err := resFile.Write(resultJson); err != nil {
		panic(fmt.Sprintf("Failed to write result: %s", err))
	}

	fmt.Printf("Done.\n")
}
