# millix-performance-test


This is a tool built to load test a Millix network.


## Setting up environment

To build this tool, we need to have Go 1.14.x installed (tested with 1.14.3)
Download link and instructions can be found here: https://golang.org/dl/

Once Go is downloaded and installed, run `go version` to verify

Afterwards, clone this repo, and run `go mod download` 

Make sure to edit the config and put the number of transactions and nodes that you want to
use in the load test.

IMPORTANT - in config you should put only nodes that you want to put the load on. For example,
if you have 10 nodes in the network but want to put load only on 3 of them, just those 3 should
be in the config. 


## Building and running
To build the tool, run the following `go build -o loader cmd/load/main.go` from the project root

Once the loader is built, it is ready to be used.

Set the following environment variables:
* CONFIG_PATH
* RESULT_PATH - path where you want the result to be written

Run the following `./loader` and keep track of the logs
