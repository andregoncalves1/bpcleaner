package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v2"
)

type Config struct {
	Azure struct {
		Cloud        string `yaml:"cloud"`
		Subscription string `yaml:"subscription"`
		// Add other Azure-related parameters here
	} `yaml:"azure"`

	Application struct {
		BlueprintsDirectoryPath       string `yaml:"blueprintsDirectoryPath"`
		UpdateBlueprintsDirectoryPath string `yaml:"updateBlueprintsDirectoryPath"`
		TargetKey                     string `yaml:"targetKey"`
		TargetValue                   string `yaml:"targetValue"`
		Env                           string `yaml:"env"`
		Dc                            string `yaml:"dc"`
		// Add other application-specific parameters here
	} `yaml:"application"`
}

// getAllYAMLFiles recursively retrieves all YAML file names in the specified directory and its subdirectories, excluding .git directory
func getAllYAMLFiles(directoryPath string) ([]string, error) {
	var fileNames []string

	// Walk through the directory and its subdirectories
	err := filepath.Walk(directoryPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip .git directory
		if info.IsDir() && info.Name() == ".git" {
			return filepath.SkipDir
		}

		// Check if it's a regular file with ".yaml" extension
		if !info.IsDir() && strings.HasSuffix(info.Name(), ".yaml") {
			fileNames = append(fileNames, path)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return fileNames, nil
}

func main() {
	// Define command-line flags
	var configFile string
	var scope string
	flag.StringVar(&configFile, "config", "", "Path to the configuration file")
	flag.StringVar(&scope, "scope", "", "blueprints, update-blueprints, all")
	flag.Parse()

	// Read configuration from the file
	config, err := readConfig(configFile)
	if err != nil {
		fmt.Printf("Error reading configuration: %v\n", err)
		return
	}

	// Login to Azure CLI if needed
	if err := azureLoginIfNeeded(config.Azure.Cloud); err != nil {
		fmt.Printf("Error logging in to Azure CLI: %v\n", err)
		return
	}

	if scope == "update-blueprints" || scope == "all" {
		// Get a list of YAML file names in the specified directory and its subdirectories
		blueprintsFileNames, err := getAllYAMLFiles(config.Application.BlueprintsDirectoryPath)
		if err != nil {
			fmt.Printf("Error getting file names: %v\n", err)
			return
		}

		updateBlueprintsFileNames, err := getAllYAMLFiles(config.Application.UpdateBlueprintsDirectoryPath)
		if err != nil {
			fmt.Printf("Error getting file names: %v\n", err)
			return
		}

		checkUpdateBlueprints(blueprintsFileNames, updateBlueprintsFileNames, config)
	}

	if scope == "blueprints" || scope == "all" {
		// Get a list of YAML file names in the specified directory and its subdirectories
		blueprintsFileNames, err := getAllYAMLFiles(config.Application.BlueprintsDirectoryPath)
		if err != nil {
			fmt.Printf("Error getting file names: %v\n", err)
			return
		}

		checkBlueprints(blueprintsFileNames, config)
	}

	if scope == "blueprints-ips" || scope == "all" {
		// Get a list of YAML file names in the specified directory and its subdirectories
		blueprintsFileNames, err := getAllYAMLFiles(config.Application.BlueprintsDirectoryPath)
		if err != nil {
			fmt.Printf("Error getting file names: %v\n", err)
			return
		}

		checkBlueprintsIPs(blueprintsFileNames, config)
	}

}

func checkBlueprints(fileNames []string, config *Config) {
	// Store cleanup guidance
	var cleanup string
	cleanup = ""

	// Loop through each file and parse to YAML
	for _, fileName := range fileNames {
		// Read the content of the file
		fileContent, err := ioutil.ReadFile(fileName)
		if err != nil {
			fmt.Printf("Error reading file %s: %v\n", fileName, err)
			continue
		}

		// Parse the file content to a YAML variable
		var yamlData map[string]interface{}
		err = yaml.Unmarshal(fileContent, &yamlData)
		if err != nil {
			fmt.Printf("Error parsing file %s to YAML: %v\n", fileName, err)
			continue
		}

		if listValue, ok := yamlData[config.Application.TargetKey]; ok {
			// Check if the target value is in the list
			if listContainsValue(listValue, config.Application.TargetValue) {
				// Check if the key "environment_specific" exists
				if environmentSpecific, ok := yamlData["environment_specific"]; ok {
					// Check if it's a list
					if environmentList, ok := environmentSpecific.([]interface{}); ok {
						// Iterate over the elements in the list
						for _, env := range environmentList {
							// Check if it's a map
							if envMap, ok := env.(map[interface{}]interface{}); ok {
								// Check if the key "environment" exists and has the value "dev"
								if envValue, ok := envMap["environment"]; ok && envValue == config.Application.Env {
									// Check if the key "environment" exists and has the value "dev"
									if envValue, ok := envMap["datacenter"]; ok && envValue == config.Application.Dc {
										// Check if the key "virtual_machines" exists and is a list
										if vmList, ok := envMap["virtual_machines"].([]interface{}); ok {
											// Iterate over virtual machines
											for _, vm := range vmList {
												// Check if it's a map
												if vmMap, ok := vm.(map[interface{}]interface{}); ok {
													// Check if the key "name" exists
													if vmName, ok := vmMap["name"]; ok {
														resourceGroup := constructResourceGroupName(envMap, yamlData)
														for i := 1; i <= vmMap["count"].(int); i++ {
															var fullVmName string
															if strings.EqualFold(vmMap["os"].(string), "windows") {
																fullVmName = fmt.Sprintf("%s-%d", vmMap["name"].(string), i)
															} else {
																fullVmName = constructVMName(envMap, yamlData, vmName.(string), i)
															}

															// Add the VM name to the slice
															fmt.Printf("File %s contains VM name: %v\n", fileName, fullVmName)

															exists, err := checkAzureVMExists(config.Azure.Subscription, resourceGroup, fullVmName)
															if err != nil {
																fmt.Printf("Resource Group %s not found. Check for cleanup blueprint %s-%s-%s\n%s\n", resourceGroup, yamlData["platform"].(string), yamlData["boundary"].(string), yamlData["name"].(string), err)
																cleanup += fmt.Sprintf("Resource Group %s not found while looking for VM %s. Check for cleanup blueprint %s-%s-%s in file %s\n", resourceGroup, fullVmName, yamlData["platform"].(string), yamlData["boundary"].(string), yamlData["name"].(string), fileName)
															} else if exists {
																fmt.Printf("Virtual machine %s exists in Azure.\n", fullVmName)
															} else {
																fmt.Printf("Virtual machine %s does not exist in Azure.\n", fullVmName)
																cleanup += fmt.Sprintf("Resource Group exists but VM %s doesn't. Check for cleanup RG and blueprint %s-%s-%s in %s\n", fullVmName, yamlData["platform"].(string), yamlData["boundary"].(string), yamlData["name"].(string), fileName)
															}
														}
													}
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	if cleanup != "" {
		fmt.Printf("\n\n#############################\n# Cleanup suggestions %s-%s\n# Blueprints\n#############################\n%s\n#############################\n", config.Application.Dc, config.Application.Env, cleanup)
	} else {
		fmt.Printf("\n\n#############################\n# Everything looks clean\n# Blueprints\n#############################\n")
	}

}

func checkBlueprintsIPs(fileNames []string, config *Config) {
	// Store cleanup guidance
	var cleanup string
	cleanup = ""

	// Loop through each file and parse to YAML
	for _, fileName := range fileNames {
		// Read the content of the file
		fileContent, err := ioutil.ReadFile(fileName)
		if err != nil {
			fmt.Printf("Error reading file %s: %v\n", fileName, err)
			continue
		}

		// Parse the file content to a YAML variable
		var yamlData map[string]interface{}
		err = yaml.Unmarshal(fileContent, &yamlData)
		if err != nil {
			fmt.Printf("Error parsing file %s to YAML: %v\n", fileName, err)
			continue
		}

		if listValue, ok := yamlData[config.Application.TargetKey]; ok {
			// Check if the target value is in the list
			if listContainsValue(listValue, config.Application.TargetValue) {
				// Check if the key "environment_specific" exists
				if environmentSpecific, ok := yamlData["environment_specific"]; ok {
					// Check if it's a list
					if environmentList, ok := environmentSpecific.([]interface{}); ok {
						// Iterate over the elements in the list
						for _, env := range environmentList {
							// Check if it's a map
							if envMap, ok := env.(map[interface{}]interface{}); ok {
								// Check if the key "environment" exists and has the value "dev"
								if envValue, ok := envMap["environment"]; ok && envValue == config.Application.Env {
									// Check if the key "environment" exists and has the value "dev"
									if envValue, ok := envMap["datacenter"]; ok && envValue == config.Application.Dc {
										// Check if the key "virtual_machines" exists and is a list
										if vmList, ok := envMap["virtual_machines"].([]interface{}); ok {
											// Iterate over virtual machines
											for _, vm := range vmList {
												// Check if it's a map
												if vmMap, ok := vm.(map[interface{}]interface{}); ok {
													// Check if the key "name" exists
													if vmName, ok := vmMap["name"]; ok {
														// Iterate over VM networks
														if vmNetworkList, ok := vmMap["networks"].([]interface{}); ok {
															for _, vmNetwork := range vmNetworkList {
																if vmNetworkMap, ok := vmNetwork.(map[interface{}]interface{}); ok {
																	if vmAddresses, ok := vmNetworkMap["address"].([]interface{}); ok {
																		//Check if number of IPs is the same as count
																		if len(vmAddresses) == vmMap["count"] {
																			fmt.Printf("Number of IP adresses and count match for %s-%s-%s-%s-%s\n", envMap["datacenter"].(string), envMap["environment"].(string), yamlData["platform"].(string), yamlData["boundary"].(string), vmMap["name"].(string))

																			resourceGroup := constructResourceGroupName(envMap, yamlData)
																			var ip_list []string
																			var ip_errors bool = false
																			//Check each IP
																			for i, vmIP := range vmAddresses {
																				var fullVmName string
																				if strings.EqualFold(vmMap["os"].(string), "windows") {
																					fullVmName = fmt.Sprintf("%s-%d", vmMap["name"].(string), i+1)
																				} else {
																					fullVmName = constructVMName(envMap, yamlData, vmName.(string), i+1)
																				}
																				ipCheck, azIP, err := checkAzureVMIP(config.Azure.Subscription, resourceGroup, fullVmName, vmIP.(string))
																				if azIP != "" {
																					ip_list = append(ip_list, azIP)
																				}
																				if err != nil {
																					fmt.Printf("IP for vm %s could not be checked. Check for cleanup blueprint %s-%s-%s\n", fullVmName, yamlData["platform"].(string), yamlData["boundary"].(string), yamlData["name"].(string))
																					cleanup += fmt.Sprintf("IP for vm %s could not be checked. Check for cleanup blueprint %s-%s-%s in file %s\n", fullVmName, yamlData["platform"].(string), yamlData["boundary"].(string), yamlData["name"].(string), fileName)
																				} else if ipCheck {
																					fmt.Printf("Virtual machine %s has correct IP in Blueprint.\n", fullVmName)
																				} else {
																					fmt.Printf("IP for vm %s does not match. Check for cleanup blueprint %s-%s-%s\n", fullVmName, yamlData["platform"].(string), yamlData["boundary"].(string), yamlData["name"].(string))
																					cleanup += fmt.Sprintf("IP for vm %s does not match. Check for cleanup blueprint %s-%s-%s in file %s\n", fullVmName, yamlData["platform"].(string), yamlData["boundary"].(string), yamlData["name"].(string), fileName)
																					ip_errors = true
																				}
																			}
																			if len(ip_list) > 0 && ip_errors {
																				fmt.Printf("Correct IP order for blueprint %s-%s-%s in the dc-env %s-%s is:\n", yamlData["platform"].(string), yamlData["boundary"].(string), yamlData["name"].(string), envMap["datacenter"].(string), envMap["environment"].(string))
																				cleanup += fmt.Sprintf("Correct IP order for blueprint %s-%s-%s in the dc-env %s-%s is:\n", yamlData["platform"].(string), yamlData["boundary"].(string), yamlData["name"].(string), envMap["datacenter"].(string), envMap["environment"].(string))
																				for _, ip := range ip_list {
																					fmt.Printf("- %s", ip)
																					cleanup += fmt.Sprintf("- %s", ip)
																				}
																			}
																		} else {
																			fmt.Printf("Number of IP adresses and count do not match for %s-%s-%s-%s-%s\n", envMap["datacenter"].(string), envMap["environment"].(string), yamlData["platform"].(string), yamlData["boundary"].(string), vmMap["name"].(string))
																			cleanup += fmt.Sprintf("Number of IP adresses and count do not match for %s-%s-%s-%s-%s\n", envMap["datacenter"].(string), envMap["environment"].(string), yamlData["platform"].(string), yamlData["boundary"].(string), vmMap["name"].(string))
																		}
																	}
																}
															}
														}
													}
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	if cleanup != "" {
		fmt.Printf("\n\n#############################\n# Cleanup suggestions %s-%s\n# Blueprint IPs\n#############################\n%s\n#############################\n", config.Application.Dc, config.Application.Env, cleanup)
	} else {
		fmt.Printf("\n\n#############################\n# Everything looks clean\n# Blueprint Ips\n#############################\n")
	}

}

func checkUpdateBlueprints(blueprintsFileNames []string, updateBlueprintsFileNames []string, config *Config) {
	// Store Blueprints YAML data in a slice
	var blueprintsAllYAMLData []map[string]interface{}

	// Loop through each file and parse to YAML
	for _, fileName := range blueprintsFileNames {
		// Read the content of the file
		fileContent, err := ioutil.ReadFile(fileName)
		if err != nil {
			fmt.Printf("Error reading file %s: %v\n", fileName, err)
			continue
		}

		// Parse the file content to a YAML variable
		var yamlData map[string]interface{}
		err = yaml.Unmarshal(fileContent, &yamlData)
		if err != nil {
			fmt.Printf("Error parsing file %s to YAML: %v\n", fileName, err)
			continue
		}

		// Add YAML data to the slice
		blueprintsAllYAMLData = append(blueprintsAllYAMLData, yamlData)
	}

	// Store Update Blueprints YAML data in a slice
	var updateBlueprintsAllYAMLData []map[string]interface{}

	// Loop through each file and parse to YAML
	for _, fileName := range updateBlueprintsFileNames {
		// Read the content of the file
		fileContent, err := ioutil.ReadFile(fileName)
		if err != nil {
			fmt.Printf("Error reading file %s: %v\n", fileName, err)
			continue
		}

		// Parse the file content to a YAML variable
		var yamlData map[string]interface{}
		err = yaml.Unmarshal(fileContent, &yamlData)
		if err != nil {
			fmt.Printf("Error parsing file %s to YAML: %v\n", fileName, err)
			continue
		}

		// Add YAML data to the slice
		updateBlueprintsAllYAMLData = append(updateBlueprintsAllYAMLData, yamlData)
	}

	// Store cleanup guidance
	var cleanup string
	cleanup = ""

	// Iterate over the YAML data and check if the target value is in the list
	for _, yamlData := range updateBlueprintsAllYAMLData {
		if listValue, ok := yamlData[config.Application.TargetKey]; ok {
			// Check if the target value is in the list
			if listContainsValue(listValue, config.Application.TargetValue) {
				// Check if the key "environment_specific" exists
				if environmentSpecific, ok := yamlData["environment_specific"]; ok {
					// Check if it's a list
					if environmentList, ok := environmentSpecific.([]interface{}); ok {
						// Iterate over the elements in the list
						for _, env := range environmentList {
							// Check if it's a map
							if envMap, ok := env.(map[interface{}]interface{}); ok {
								// Check if the key "environment" exists and has the value "dev"
								if envValue, ok := envMap["environment"]; ok && envValue == config.Application.Env {
									// Check if the key "environment" exists and has the value "dev"
									if envValue, ok := envMap["datacenter"]; ok && envValue == config.Application.Dc {
										// Check if the key "virtual_machines" exists and is a list
										if vmList, ok := envMap["virtual_machines"].([]interface{}); ok {
											// Iterate over virtual machines
											for _, vm := range vmList {
												// Check if it's a map
												if vmMap, ok := vm.(map[interface{}]interface{}); ok {
													// Check if the key "infrastructure_blueprint" exists
													if envValue, ok := vmMap["infrastructure_blueprint"]; ok {
														exists, _ := checkBlueprintFromUpdateBlueprint(envValue.(string), config, blueprintsAllYAMLData)
														if exists {
															fmt.Printf("Update blueprint %s-%s-%s-%s-%s has a matching blueprint.\n", yamlData["platform"].(string), yamlData["boundary"].(string), yamlData["name"].(string), config.Application.Dc, config.Application.Env)
														} else {
															fmt.Printf("Update blueprint %s-%s-%s-%s-%s does not have a matching blueprint.\n", yamlData["platform"].(string), yamlData["boundary"].(string), yamlData["name"].(string), config.Application.Dc, config.Application.Env)
															cleanup += fmt.Sprintf("Update blueprint %s-%s-%s-%s-%s does not have a matching blueprint.\n", yamlData["platform"].(string), yamlData["boundary"].(string), yamlData["name"].(string), config.Application.Dc, config.Application.Env)
														}
													}
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
	if cleanup != "" {
		fmt.Printf("\n\n#############################\n# Cleanup suggestions %s-%s\n# Update Blueprints\n#############################\n%s\n#############################\n", config.Application.Dc, config.Application.Env, cleanup)
	} else {
		fmt.Printf("\n\n#############################\n# Everything looks clean\n# Update Blueprints\n#############################\n")
	}
}

func checkBlueprintFromUpdateBlueprint(blueprintPBN string, config *Config, blueprintsAllYAMLData []map[string]interface{}) (bool, error) {

	// Iterate over the YAML data and check if the target value is in the list
	for _, yamlData := range blueprintsAllYAMLData {
		if platform, ok := yamlData["platform"].(string); ok {
			if boundary, ok := yamlData["boundary"].(string); ok {
				if name, ok := yamlData["name"].(string); ok {
					pbn := fmt.Sprintf("%s-%s-%s", platform, boundary, name)
					if strings.EqualFold(pbn, blueprintPBN) {
						if listValue, ok := yamlData[config.Application.TargetKey]; ok {
							// Check if the target value is in the list
							if listContainsValue(listValue, config.Application.TargetValue) {
								// Check if the key "environment_specific" exists
								if environmentSpecific, ok := yamlData["environment_specific"]; ok {
									// Check if it's a list
									if environmentList, ok := environmentSpecific.([]interface{}); ok {
										// Iterate over the elements in the list
										for _, env := range environmentList {
											// Check if it's a map
											if envMap, ok := env.(map[interface{}]interface{}); ok {
												// Check if the key "environment" exists and has the value "dev"
												if envValue, ok := envMap["environment"]; ok && envValue == config.Application.Env {
													// Check if the key "environment" exists and has the value "dev"
													if envValue, ok := envMap["datacenter"]; ok && envValue == config.Application.Dc {
														return true, nil
													}
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
	return false, nil

}

func constructResourceGroupName(envMap map[interface{}]interface{}, yamlData map[string]interface{}) string {
	environment, _ := envMap["environment"].(string)
	datacenter, _ := envMap["datacenter"].(string)
	platform, _ := yamlData["platform"].(string)
	boundary, _ := yamlData["boundary"].(string)
	name, _ := yamlData["name"].(string)

	// Construct the VM name using the desired format
	vmName := fmt.Sprintf("%s-%s-%s-%s-%s", datacenter, environment, platform, boundary, name)

	return vmName
}

func constructVMName(envMap map[interface{}]interface{}, yamlData map[string]interface{}, name string, count int) string {
	environment, _ := envMap["environment"].(string)
	datacenter, _ := envMap["datacenter"].(string)
	platform, _ := yamlData["platform"].(string)
	boundary, _ := yamlData["boundary"].(string)

	// Construct the VM name using the desired format
	vmName := fmt.Sprintf("%s-%s-%s-%s-%s-%d", datacenter, environment, platform, boundary, name, count)

	return vmName
}

// readConfig reads the configuration from the specified file
func readConfig(configFile string) (*Config, error) {
	if configFile == "" {
		return nil, fmt.Errorf("config file path not provided")
	}

	// Read the content of the configuration file
	fileContent, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("error reading config file: %v", err)
	}

	// Parse the content to a Config struct
	var config Config
	err = yaml.Unmarshal(fileContent, &config)
	if err != nil {
		return nil, fmt.Errorf("error parsing config file: %v", err)
	}

	return &config, nil
}

// listContainsValue checks if a value exists in a list (slice)
func listContainsValue(listValue interface{}, targetValue string) bool {
	switch v := listValue.(type) {
	case []interface{}:
		// Iterate over the elements in the list
		for _, item := range v {
			// Check if the item is a string and equal to the target value
			if str, ok := item.(string); ok && str == targetValue {
				return true
			}
		}
	}
	return false
}

// checkAzureVMExists checks if a virtual machine with the given name exists in Azure using Azure CLI
func checkAzureVMExists(subscription, resourceGroup, vmName string) (bool, error) {
	// Use Azure CLI to check VM existence with specified resource group
	cmd := exec.Command("az", "vm", "show", "--name", vmName, "--resource-group", resourceGroup, "--subscription", subscription)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if the VM not found error occurred
		if strings.Contains(string(output), "ResourceNotFound") {
			return false, nil
		}
		return false, fmt.Errorf("error executing Azure CLI command: %v\nOutput: %s", err, output)
	}

	return true, nil
}

// checkAzureVMExists checks if a virtual machine with the given name exists in Azure using Azure CLI
func checkAzureVMIP(subscription, resourceGroup, vmName string, ip string) (bool, string, error) {
	// Use Azure CLI to check VM existence with specified resource group
	cmd := exec.Command("az", "vm", "show", "--name", vmName, "--resource-group", resourceGroup, "--subscription", subscription, "-d", "--query", "\"privateIps\"", "--out", "tsv")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, "", fmt.Errorf("error executing Azure CLI command: %v\nOutput: %s", err, output)
	}

	if strings.Contains(string(output), ip) {
		return true, string(output), nil
	}
	return false, string(output), nil
}

// azureLoginIfNeeded logs in to Azure CLI if not already logged in
func azureLoginIfNeeded(azureCloud string) error {
	// Azure CLI set cloud
	setCloudCmd := exec.Command("az", "cloud", "set", "--name", azureCloud)
	setCloudOutput, setCloudErr := setCloudCmd.CombinedOutput()
	if setCloudErr != nil {
		return fmt.Errorf("error executing Azure CLI cloud set command: %v\nOutput: %s", setCloudErr, setCloudOutput)
	}

	// Check if Azure CLI is already logged in
	cmd := exec.Command("az", "account", "show")
	_, err := cmd.CombinedOutput()
	if err == nil {
		// Azure CLI is already logged in
		return nil
	}

	// Azure CLI is not logged in, perform login
	loginCmd := exec.Command("az", "login")
	loginOutput, loginErr := loginCmd.CombinedOutput()
	if loginErr != nil {
		return fmt.Errorf("error executing Azure CLI login command: %v\nOutput: %s", loginErr, loginOutput)
	}

	return nil
}
