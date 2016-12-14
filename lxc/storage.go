package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"sort"
	"strings"
	"syscall"

	"github.com/olekukonko/tablewriter"
	"gopkg.in/yaml.v2"

	"github.com/lxc/lxd"
	"github.com/lxc/lxd/shared"
	"github.com/lxc/lxd/shared/i18n"
	"github.com/lxc/lxd/shared/termios"
)

type storageCmd struct {
}

func (c *storageCmd) showByDefault() bool {
	return true
}

func (c *storageCmd) networkEditHelp() string {
	return i18n.G(
		`### This is a yaml representation of the network.
### Any line starting with a '# will be ignored.
###
### A network consists of a set of configuration items.
###
### An example would look like:
### name: lxdbr0
### config:
###   ipv4.address: 10.62.42.1/24
###   ipv4.nat: true
###   ipv6.address: fd00:56ad:9f7a:9800::1/64
###   ipv6.nat: true
### managed: true
### type: bridge
###
### Note that only the configuration can be changed.`)
}

func (c *storageCmd) usage() string {
	return i18n.G(
		`Manage networks.

lxc network list                               List available networks.
lxc network show <network>                     Show details of a network.
lxc network create <network> [key=value]...    Create a network.
lxc network get <network> <key>                Get network configuration.
lxc network set <network> <key> <value>        Set network configuration.
lxc network unset <network> <key>              Unset network configuration.
lxc network delete <network>                   Delete a network.
lxc network edit <network>
    Edit network, either by launching external editor or reading STDIN.
    Example: lxc network edit <network> # launch editor
             cat network.yaml | lxc network edit <network> # read from network.yaml

lxc network attach <network> <container> [device name]
lxc network attach-profile <network> <profile> [device name]

lxc network detach <network> <container> [device name]
lxc network detach-profile <network> <container> [device name]
`)
}

func (c *storageCmd) flags() {}

func (c *storageCmd) run(config *lxd.Config, args []string) error {
	if len(args) < 1 {
		return errArgs
	}

	if args[0] == "list" {
		return c.doStoragePoolsList(config, args)
	}

	remote, pool := config.ParseRemoteAndContainer(args[1])
	client, err := lxd.NewClient(config, remote)
	if err != nil {
		return err
	}

	switch args[0] {
	// case "attach":
	// 	return c.doNetworkAttach(client, network, args[2:])
	// case "attach-profile":
	// 	return c.doNetworkAttachProfile(client, network, args[2:])
	case "create":
		if len(args) < 3 {
			return errArgs
		}
		driver := strings.Join(args[2:3], "")
		return c.doStoragePoolCreate(client, pool, driver, args[3:])
	case "delete":
		return c.doStoragePoolDelete(client, pool)
	// case "detach":
	// 	return c.doNetworkDetach(client, network, args[2:])
	// case "detach-profile":
	// 	return c.doNetworkDetachProfile(client, network, args[2:])
	// case "edit":
	// 	return c.doNetworkEdit(client, network)
	case "get":
		if len(args) < 2 {
			return errArgs
		}
		return c.doStoragePoolGet(client, pool, args[2:])
	// case "set":
	// 	return c.doNetworkSet(client, network, args[2:])
	// case "unset":
	// 	return c.doNetworkSet(client, network, args[2:])
	// case "show":
	// 	return c.doNetworkShow(client, network)
	default:
		return errArgs
	}
}

func (c *storageCmd) doNetworkAttach(client *lxd.Client, name string, args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return errArgs
	}

	container := args[0]
	devName := name
	if len(args) > 1 {
		devName = args[1]
	}

	network, err := client.NetworkGet(name)
	if err != nil {
		return err
	}

	nicType := "macvlan"
	if network.Type == "bridge" {
		nicType = "bridged"
	}

	props := []string{fmt.Sprintf("nictype=%s", nicType), fmt.Sprintf("parent=%s", name)}
	resp, err := client.ContainerDeviceAdd(container, devName, "nic", props)
	if err != nil {
		return err
	}

	return client.WaitForSuccess(resp.Operation)
}

func (c *storageCmd) doNetworkAttachProfile(client *lxd.Client, name string, args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return errArgs
	}

	profile := args[0]
	devName := name
	if len(args) > 1 {
		devName = args[1]
	}

	network, err := client.NetworkGet(name)
	if err != nil {
		return err
	}

	nicType := "macvlan"
	if network.Type == "bridge" {
		nicType = "bridged"
	}

	props := []string{fmt.Sprintf("nictype=%s", nicType), fmt.Sprintf("parent=%s", name)}
	_, err = client.ProfileDeviceAdd(profile, devName, "nic", props)
	return err
}

func (c *storageCmd) doStoragePoolCreate(client *lxd.Client, name string, driver string, args []string) error {
	config := map[string]string{}

	config["driver"] = driver

	for i := 0; i < len(args); i++ {
		entry := strings.SplitN(args[i], "=", 2)
		if len(entry) < 2 {
			return errArgs
		}
		config[entry[0]] = entry[1]
	}

	err := client.StoragePoolCreate(name, config)
	if err == nil {
		fmt.Printf(i18n.G("Storage pool %s created")+"\n", name)
	}

	return err
}

func (c *storageCmd) doNetworkDetach(client *lxd.Client, name string, args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return errArgs
	}

	containerName := args[0]
	devName := ""
	if len(args) > 1 {
		devName = args[1]
	}

	container, err := client.ContainerInfo(containerName)
	if err != nil {
		return err
	}

	if devName == "" {
		for n, d := range container.Devices {
			if d["type"] == "nic" && d["parent"] == name {
				if devName != "" {
					return fmt.Errorf(i18n.G("More than one device matches, specify the device name."))
				}

				devName = n
			}
		}
	}

	if devName == "" {
		return fmt.Errorf(i18n.G("No device found for this network"))
	}

	device, ok := container.Devices[devName]
	if !ok {
		return fmt.Errorf(i18n.G("The specified device doesn't exist"))
	}

	if device["type"] != "nic" || device["parent"] != name {
		return fmt.Errorf(i18n.G("The specified device doesn't match the network"))
	}

	resp, err := client.ContainerDeviceDelete(containerName, devName)
	if err != nil {
		return err
	}

	return client.WaitForSuccess(resp.Operation)
}

func (c *storageCmd) doNetworkDetachProfile(client *lxd.Client, name string, args []string) error {
	if len(args) < 1 || len(args) > 2 {
		return errArgs
	}

	profileName := args[0]
	devName := ""
	if len(args) > 1 {
		devName = args[1]
	}

	profile, err := client.ProfileConfig(profileName)
	if err != nil {
		return err
	}

	if devName == "" {
		for n, d := range profile.Devices {
			if d["type"] == "nic" && d["parent"] == name {
				if devName != "" {
					return fmt.Errorf(i18n.G("More than one device matches, specify the device name."))
				}

				devName = n
			}
		}
	}

	if devName == "" {
		return fmt.Errorf(i18n.G("No device found for this network"))
	}

	device, ok := profile.Devices[devName]
	if !ok {
		return fmt.Errorf(i18n.G("The specified device doesn't exist"))
	}

	if device["type"] != "nic" || device["parent"] != name {
		return fmt.Errorf(i18n.G("The specified device doesn't match the network"))
	}

	_, err = client.ProfileDeviceDelete(profileName, devName)
	return err
}

func (c *storageCmd) doStoragePoolDelete(client *lxd.Client, name string) error {
	err := client.StoragePoolDelete(name)
	if err == nil {
		fmt.Printf(i18n.G("Storage pool %s deleted")+"\n", name)
	}

	return err
}

func (c *storageCmd) doNetworkEdit(client *lxd.Client, name string) error {
	// If stdin isn't a terminal, read text from it
	if !termios.IsTerminal(int(syscall.Stdin)) {
		contents, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			return err
		}

		newdata := shared.NetworkConfig{}
		err = yaml.Unmarshal(contents, &newdata)
		if err != nil {
			return err
		}
		return client.NetworkPut(name, newdata)
	}

	// Extract the current value
	network, err := client.NetworkGet(name)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(&network)
	if err != nil {
		return err
	}

	// Spawn the editor
	content, err := shared.TextEditor("", []byte(c.networkEditHelp()+"\n\n"+string(data)))
	if err != nil {
		return err
	}

	for {
		// Parse the text received from the editor
		newdata := shared.NetworkConfig{}
		err = yaml.Unmarshal(content, &newdata)
		if err == nil {
			err = client.NetworkPut(name, newdata)
		}

		// Respawn the editor
		if err != nil {
			fmt.Fprintf(os.Stderr, i18n.G("Config parsing error: %s")+"\n", err)
			fmt.Println(i18n.G("Press enter to open the editor again"))

			_, err := os.Stdin.Read(make([]byte, 1))
			if err != nil {
				return err
			}

			content, err = shared.TextEditor("", content)
			if err != nil {
				return err
			}
			continue
		}
		break
	}
	return nil
}

func (c *storageCmd) doStoragePoolGet(client *lxd.Client, name string, args []string) error {
	// we shifted @args so so it should read "<key>"
	if len(args) != 1 {
		return errArgs
	}

	resp, err := client.StoragePoolGet(name)
	if err != nil {
		return err
	}

	for k, v := range resp.Config {
		if k == args[0] {
			fmt.Printf("%s\n", v)
		}
	}
	return nil
}

func (c *storageCmd) doStoragePoolsList(config *lxd.Config, args []string) error {
	var remote string
	if len(args) > 1 {
		var name string
		remote, name = config.ParseRemoteAndContainer(args[1])
		if name != "" {
			return fmt.Errorf(i18n.G("Cannot provide container name to list"))
		}
	} else {
		remote = config.DefaultRemote
	}

	client, err := lxd.NewClient(config, remote)
	if err != nil {
		return err
	}

	pools, err := client.ListStoragePools()
	if err != nil {
		return err
	}

	data := [][]string{}
	for _, pool := range pools {
		data = append(data, []string{pool.Name, pool.Driver, pool.Config["size"], pool.Config["source"]})
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetAutoWrapText(false)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetRowLine(true)
	table.SetHeader([]string{
		i18n.G("NAME"),
		i18n.G("DRIVER"),
		i18n.G("SIZE"),
		i18n.G("SOURCE")})
	sort.Sort(byName(data))
	table.AppendBulk(data)
	table.Render()

	return nil
}

func (c *storageCmd) doNetworkSet(client *lxd.Client, name string, args []string) error {
	// we shifted @args so so it should read "<key> [<value>]"
	if len(args) < 1 {
		return errArgs
	}

	network, err := client.NetworkGet(name)
	if err != nil {
		return err
	}

	key := args[0]
	var value string
	if len(args) < 2 {
		value = ""
	} else {
		value = args[1]
	}

	if !termios.IsTerminal(int(syscall.Stdin)) && value == "-" {
		buf, err := ioutil.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("Can't read from stdin: %s", err)
		}
		value = string(buf[:])
	}

	network.Config[key] = value

	return client.NetworkPut(name, network)
}

func (c *storageCmd) doNetworkShow(client *lxd.Client, name string) error {
	network, err := client.NetworkGet(name)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(&network)
	fmt.Printf("%s", data)

	return nil
}
