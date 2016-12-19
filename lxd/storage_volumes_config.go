package main

import (
	"fmt"
	"strings"

	"github.com/lxc/lxd/shared"
)

var storageVolumeConfigKeys = map[string]func(value string) error{
	"block.mount_options":  shared.IsAny,
	"block.filesystem":     shared.IsAny,
	"size":                 shared.IsAny,
	"zfs.use_refquota":     shared.IsBool,
	"zfs.remove_snapshots": shared.IsBool,
}

func storageVolumeValidateConfig(name string, config map[string]string, parentPool *shared.StoragePoolConfig) error {
	for key, val := range config {
		// User keys are not validated.
		if strings.HasPrefix(key, "user.") {
			continue
		}

		// Validate storage volume config keys.
		validator, ok := storageVolumeConfigKeys[key]
		if !ok {
			return fmt.Errorf("Invalid storage volume configuration key: %s", key)
		}

		err := validator(val)
		if err != nil {
			return err
		}

		// TODO: way retrieve the parent volume or pass in the parent volume's
		if parentPool.Config["driver"] != "zfs" || parentPool.Config["driver"] == "dir" {
			if config["zfs.use_refquota"] != "" {
				return fmt.Errorf("Key volume.zfs.use_refquota cannot be used with non zfs storage volumes.")
			}

			if config["zfs.remove_snapshots"] != "" {
				return fmt.Errorf("Key volume.zfs.remove_snapshots cannot be used with non zfs storage volumes.")
			}
		}

		if parentPool.Config["driver"] == "dir" {
			if config["block.mount_options"] != "" {
				return fmt.Errorf("Key block.mount_options cannot be used with dir storage volumes.")
			}

			if config["block.filesystem"] != "" {
				return fmt.Errorf("Key block.filesystem cannot be used with dir storage volumes.")
			}

			if config["size"] != "" {
				return fmt.Errorf("Key size cannot be used with dir storage volumes.")
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
