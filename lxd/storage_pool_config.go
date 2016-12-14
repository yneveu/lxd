package main

import (
	"fmt"
	"strings"

	"github.com/lxc/lxd/shared"
)

var storagePoolConfigKeys = map[string]func(value string) error{
	"driver": func(value string) error {
		return shared.IsOneOf(value, []string{"zfs"})
	},
	"source": shared.IsAny,
	"size":   shared.IsAny,
	"volume.block.mount_options":  shared.IsAny,
	"volume.block.filesystem":     shared.IsAny,
	"volume.size":                 shared.IsInt64,
	"volume.zfs.use_refquota":     shared.IsBool,
	"volume.zfs.remove_snapshots": shared.IsBool,
	"zfs.pool_name":               shared.IsAny,
}

func storagePoolValidateConfig(name string, config map[string]string) error {
	driver := config["driver"]
	if driver == "" {
		return fmt.Errorf("You must specify a driver to use for the storage pool.")
	}

	if config["source"] == "" {
		config["source"] = shared.VarPath(name)
	}

	for key, val := range config {
		// User keys are not validated.
		if strings.HasPrefix(key, "user.") {
			continue
		}

		// Validate storage pool config keys.
		validator, ok := storagePoolConfigKeys[key]
		if !ok {
			return fmt.Errorf("Invalid storage pool configuration key: %s", key)
		}

		err := validator(val)
		if err != nil {
			return err
		}

		if driver != "zfs" {
			if config["volume.zfs.use_refquota"] != "" {
				return fmt.Errorf("Key volume.zfs.use_refquota cannot be used with non zfs storage pools.")
			}

			if config["volume.zfs.remove_snapshots"] != "" {
				return fmt.Errorf("Key volume.zfs.remove_snapshots cannot be used with non zfs storage pools.")
			}

			if config["zfs.pool_name"] != "" {
				return fmt.Errorf("Key zfs.pool_name cannot be used with non zfs storage pools.")
			}
		}

		if key == "size" && val != "" {
			_, err := shared.ParseByteSizeString(val)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
