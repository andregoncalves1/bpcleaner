# BPCleaner

This tool looks for cleanups that can be done.
- Blueprints that have some team as maintainers and checks if the VMs exist in Azure
- Update-blueprints that don't have a matching blueprint

## Dependencies

- Golang installed
- Azure CLI installed

## How to run

```
go run main.go -config <config file> -scope <blueprints|update-blueprints|all>
```