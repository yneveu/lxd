package main

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"

	"github.com/lxc/lxd/shared"
)

// Get all storage volumes.
func dbStorageVolumes(db *sql.DB) ([]string, error) {
	q := fmt.Sprintf("SELECT volume_name FROM storage_volumes")
	inargs := []interface{}{}
	var name string
	outfmt := []interface{}{name}
	result, err := dbQueryScan(db, q, inargs, outfmt)
	if err != nil {
		return []string{}, err
	}

	response := []string{}
	for _, r := range result {
		response = append(response, r[0].(string))
	}

	return response, nil
}

// Get a single storage volume.
func dbStorageVolumeGet(db *sql.DB, volume string) (int64, *shared.StorageVolumeConfig, error) {
	id := int64(-1)

	q := "SELECT id FROM storage_volumes WHERE volume_name=?"
	arg1 := []interface{}{volume}
	arg2 := []interface{}{&id}
	err := dbQueryRowScan(db, q, arg1, arg2)
	if err != nil {
		return -1, nil, err
	}

	config, err := dbStorageVolumeConfigGet(db, id)
	if err != nil {
		return -1, nil, err
	}

	return id, &shared.StorageVolumeConfig{
		PoolName: volume,
		Config:   config,
	}, nil
}

// Get config of a storage volume.
func dbStorageVolumeConfigGet(db *sql.DB, id int64) (map[string]string, error) {
	var key, value string
	query := `
        SELECT
            key, value
        FROM storage_volumes_config
		WHERE storage_volume_id=?`
	inargs := []interface{}{id}
	outfmt := []interface{}{key, value}
	results, err := dbQueryScan(db, query, inargs, outfmt)
	if err != nil {
		return nil, fmt.Errorf("Failed to get storage volume '%d'", id)
	}

	if len(results) == 0 {
		/*
		 * If we didn't get any rows here, let's check to make sure the
		 * storage volume really exists; if it doesn't, let's send back
		 * a 404.
		 */
		query := "SELECT id FROM storage_volumes WHERE id=?"
		var r int
		results, err := dbQueryScan(db, query, []interface{}{id}, []interface{}{r})
		if err != nil {
			return nil, err
		}

		if len(results) == 0 {
			return nil, NoSuchObjectError
		}
	}

	config := map[string]string{}

	for _, r := range results {
		key = r[0].(string)
		value = r[1].(string)

		config[key] = value
	}

	return config, nil
}

// Create new storage volume table.
func dbStorageVolumeCreate(db *sql.DB, name string, config map[string]string) (int64, error) {
	tx, err := dbBegin(db)
	if err != nil {
		return -1, err
	}

	result, err := tx.Exec("INSERT INTO storage_volumes (volume_name) VALUES (?)", name)
	if err != nil {
		tx.Rollback()
		return -1, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		tx.Rollback()
		return -1, err
	}

	err = dbStorageVolumeConfigAdd(tx, id, config)
	if err != nil {
		tx.Rollback()
		return -1, err
	}

	err = txCommit(tx)
	if err != nil {
		return -1, err
	}

	return id, nil
}

// Add new storage volume config into database.
func dbStorageVolumeConfigAdd(tx *sql.Tx, id int64, config map[string]string) error {
	str := fmt.Sprintf("INSERT INTO storage_volumes_config (storage_volume_id, key, value) VALUES(?, ?, ?)")
	stmt, err := tx.Prepare(str)
	defer stmt.Close()

	for k, v := range config {
		if v == "" {
			continue
		}

		_, err = stmt.Exec(id, k, v)
		if err != nil {
			return err
		}
	}

	return nil
}

// Update storage volume.
func dbStorageVolumeUpdate(db *sql.DB, name string, config map[string]string) error {
	id, _, err := dbStorageVolumeGet(db, name)
	if err != nil {
		return err
	}

	tx, err := dbBegin(db)
	if err != nil {
		return err
	}

	err = dbStorageVolumeConfigClear(tx, id)
	if err != nil {
		tx.Rollback()
		return err
	}

	err = dbStorageVolumeConfigAdd(tx, id, config)
	if err != nil {
		tx.Rollback()
		return err
	}

	return txCommit(tx)
}

// Delete storage volume config.
func dbStorageVolumeConfigClear(tx *sql.Tx, id int64) error {
	_, err := tx.Exec("DELETE FROM storage_volumes_config WHERE storage_volume_id=?", id)
	if err != nil {
		return err
	}

	return nil
}

// Delete storage volume.
func dbStorageVolumeDelete(db *sql.DB, name string) error {
	id, _, err := dbStorageVolumeGet(db, name)
	if err != nil {
		return err
	}

	tx, err := dbBegin(db)
	if err != nil {
		return err
	}

	_, err = tx.Exec("DELETE FROM storage_volumes WHERE id=?", id)
	if err != nil {
		tx.Rollback()
		return err
	}

	err = dbStorageVolumeConfigClear(tx, id)
	if err != nil {
		tx.Rollback()
		return err
	}

	return txCommit(tx)
}

// Rename storage volume.
func dbStorageVolumeRename(db *sql.DB, oldName string, newName string) error {
	id, _, err := dbStorageVolumeGet(db, oldName)
	if err != nil {
		return err
	}

	tx, err := dbBegin(db)
	if err != nil {
		return err
	}

	_, err = tx.Exec("UPDATE storage_volumes SET volume_name=? WHERE id=?", newName, id)
	if err != nil {
		tx.Rollback()
		return err
	}

	return txCommit(tx)
}

// func dbStorageVolumeGetVolume(db *sql.DB, devName string) (int64, *shared.NetworkConfig, error) {
// 	id := int64(-1)
// 	name := ""
// 	value := ""
//
// 	q := "SELECT storage_volumes.id, storage_volumes.name, storage_volumes_config.value FROM storage_volumes LEFT JOIN storage_volumes_config ON networks.id=storage_volumes_config.network_id WHERE storage_volumes_config.key=\"bridge.external_interfaces\""
// 	arg1 := []interface{}{}
// 	arg2 := []interface{}{id, name, value}
// 	result, err := dbQueryScan(db, q, arg1, arg2)
// 	if err != nil {
// 		return -1, nil, err
// 	}
//
// 	for _, r := range result {
// 		for _, entry := range strings.Split(r[2].(string), ",") {
// 			entry = strings.TrimSpace(entry)
//
// 			if entry == devName {
// 				id = r[0].(int64)
// 				name = r[1].(string)
// 			}
// 		}
// 	}
//
// 	if id == -1 {
// 		return -1, nil, fmt.Errorf("No network found for interface: %s", devName)
// 	}
//
// 	config, err := dbStoragePoolConfigGet(db, id)
// 	if err != nil {
// 		return -1, nil, err
// 	}
//
// 	return id, &shared.NetworkConfig{
// 		Name:    name,
// 		Managed: true,
// 		Type:    "bridge",
// 		Config:  config,
// 	}, nil
// }
