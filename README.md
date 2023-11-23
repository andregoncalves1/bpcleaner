#BPCleaner

This tool looks for all the blueprints that have some team as maintainers and checks if the VMs exist in Azure

#Dependencies

- Golang installed
- Azure CLI installed

#How to run

go run main.go -config <config file> -scope <blueprints|update-blueprints|all>