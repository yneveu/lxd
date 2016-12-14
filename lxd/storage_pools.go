package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/gorilla/mux"
	"github.com/lxc/lxd/shared"
)

// /1.0/storage-pools
func storagePoolsGet(d *Daemon, r *http.Request) Response {
	recursionStr := r.FormValue("recursion")
	recursion, err := strconv.Atoi(recursionStr)
	if err != nil {
		recursion = 0
	}

	pools, err := storagePoolsGetPools(d)
	if err != nil {
		return InternalError(err)
	}

	resultString := []string{}
	resultMap := []shared.StoragePoolConfig{}
	for _, pool := range pools {
		if recursion == 0 {
			resultString = append(resultString, fmt.Sprintf("/%s/storage-pools/%s", shared.APIVersion, pool))
		} else {
			net, err := doStoragePoolGet(d, pool)
			if err != nil {
				continue
			}
			resultMap = append(resultMap, net)
		}
	}

	if recursion == 0 {
		return SyncResponse(true, resultString)
	}

	return SyncResponse(true, resultMap)
}

func storagePoolsPost(d *Daemon, r *http.Request) Response {
	req := shared.StoragePoolConfig{}

	// Parse the request
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		return BadRequest(err)
	}

	// Sanity checks
	if req.Name == "" {
		return BadRequest(fmt.Errorf("No name provided"))
	}

	err = storagePoolValidName(req.Name)
	if err != nil {
		return BadRequest(err)
	}

	// Check that the storage pool does not already exist.
	pools, err := dbStoragePools(d.db)
	if err != nil {
		return InternalError(err)
	}

	if shared.StringInSlice(req.Name, pools) {
		return Conflict
	}

	// Make sure that we don't pass a nil to the next function.
	if req.Config == nil {
		req.Config = map[string]string{}
	}

	// Validate the requested storage pool configuration.
	err = storagePoolValidateConfig(req.Name, req.Config)
	if err != nil {
		return BadRequest(err)
	}

	// Create the database entry for the storage pool.
	_, err = dbStoragePoolCreate(d.db, req.Name, req.Config)
	if err != nil {
		return InternalError(
			fmt.Errorf("Error inserting %s into database: %s", req.Name, err))
	}

	// Load the storage pool from teh database.
	s, err := storagePoolLoadByName(d, req.Name)
	if err != nil {
		return InternalError(err)
	}

	// Create storage pool.
	err = s.storagePoolCreate()
	if err != nil {
		// s.storagePoolDelete()
		return InternalError(err)
	}

	return SyncResponseLocation(true, nil, fmt.Sprintf("/%s/storage-pools/%s", shared.APIVersion, req.Name))
}

var storagePoolsCmd = Command{name: "storage-pools", get: storagePoolsGet, post: storagePoolsPost}

// /1.0/storage-pools/<pool name>
func storagePoolGet(d *Daemon, r *http.Request) Response {
	return nil
}

func storagePoolPost(d *Daemon, r *http.Request) Response {
	return nil
}

func storagePoolPut(d *Daemon, r *http.Request) Response {
	return nil
}

func storagePoolPatch(d *Daemon, r *http.Request) Response {
	return nil
}

func storagePoolDelete(d *Daemon, r *http.Request) Response {
	name := mux.Vars(r)["name"]

	// Get the existing network
	s, err := storagePoolLoadByName(d, name)
	if err != nil {
		return NotFound
	}

	if s.config["driver"] == "zfs" {
		if err := s.zfsPoolDelete(); err != nil {
			return InternalError(err)
		}
	}

	if err := dbStoragePoolDelete(d.db, name); err != nil {
		return InternalError(err)
	}

	return EmptySyncResponse
}

var storagePoolCmd = Command{name: "storage-pools/{name}", get: storagePoolGet, post: storagePoolPost, put: storagePoolPut, patch: storagePoolPatch, delete: storagePoolDelete}

// /1.0/storage-pools/<pool-name>/volumes
func storageVolumesGet(d *Daemon, r *http.Request) Response {
	return nil
}

func storageVolumesPost(d *Daemon, r *http.Request) Response {
	return nil
}

var storageVolumesCmd = Command{name: "storage-pools/{name}/volumes", get: storageVolumesGet, post: storageVolumesPost}

// /1.0/storage-pools/<pool-name>/volumes/volume name>
func storageVolumeGet(d *Daemon, r *http.Request) Response {
	return nil
}

func storageVolumePost(d *Daemon, r *http.Request) Response {
	return nil
}

func storageVolumePut(d *Daemon, r *http.Request) Response {
	return nil
}

func storageVolumePatch(d *Daemon, r *http.Request) Response {
	return nil
}

func storageVolumeDelete(d *Daemon, r *http.Request) Response {
	return nil
}

var storageVolumeCmd = Command{name: "storage-pools/{name}/volumes/{name}", get: storageVolumeGet, post: storageVolumePost, put: storageVolumePut, patch: storageVolumePatch, delete: storageVolumeDelete}

// The storage pool structs and functions
type storagePool struct {
	// Properties
	daemon *Daemon
	id     int64
	name   string

	// config
	config map[string]string
}

func storagePoolLoadByName(d *Daemon, name string) (*storagePool, error) {
	id, dbInfo, err := dbStoragePoolGet(d.db, name)
	if err != nil {
		return nil, err
	}

	s := storagePool{daemon: d, id: id, name: name, config: dbInfo.Config}

	return &s, nil
}

func (s *storagePool) storagePoolCreate() error {
	size := ""
	if s.config["driver"] != "dir" {
		if s.config["size"] == "" {
			st := syscall.Statfs_t{}
			err := syscall.Statfs(shared.VarPath(), &st)
			if err != nil {
				return fmt.Errorf("couldn't statfs %s: %s", shared.VarPath(), err)
			}

			/* choose 15 GB < x < 100GB, where x is 20% of the disk size */
			sz := uint64(st.Frsize) * st.Blocks / (1024 * 1024 * 1024) / 5
			if sz > 100 {
				sz = 100
			}
			if sz < 15 {
				sz = 15
			}

			size = strconv.FormatUint(sz, 10) + "GB"
			s.config["size"] = size
		}

		if err := dbStoragePoolUpdate(s.daemon.db, s.name, s.config); err != nil {
			return fmt.Errorf("Failed to update database")
		}
	}

	if s.config["driver"] == "zfs" {
		return s.zfsPoolCreate()
	}

	return nil
}

func (s *storagePool) zfsPoolCreate() error {
	vdev := s.config["source"]
	if vdev == "" {
		vdev = shared.VarPath(s.name)
	}

	out, err := exec.LookPath("zfs")
	if err != nil || len(out) == 0 {
		return fmt.Errorf("The 'zfs' tool isn't available")
	}

	if !filepath.IsAbs(vdev) {
		// Probably a zpool or zfs dataset.
		if err := zfsPoolCheck(vdev); err != nil {
			return err
		}

		// Confirm that the pool is empty.
		subvols, err := zfsPoolListSubvolumes(vdev)
		if err != nil {
			return err
		}

		if len(subvols) > 0 {
			return fmt.Errorf("Provided ZFS pool (or dataset) isn't empty")
		}

		_ = loadModule("zfs")

		return nil
	} else {
		if !shared.IsBlockdevPath(vdev) {
			// This is likely a loop file.
			f, err := os.Create(vdev)
			if err != nil {
				return fmt.Errorf("Failed to open %s: %s", vdev, err)
			}

			err = f.Chmod(0600)
			if err != nil {
				return fmt.Errorf("Failed to chmod %s: %s", vdev, err)
			}

			size := int64(0)
			if s.config["size"] != "" {
				size, err = shared.ParseByteSizeString(s.config["size"])
				if err != nil {
					return err
				}
			}
			err = f.Truncate(size)
			if err != nil {
				return fmt.Errorf("Failed to create sparse file %s: %s", vdev, err)
			}

			err = f.Close()
			if err != nil {
				return fmt.Errorf("Failed to close %s: %s", vdev, err)
			}
		}
	}

	zpoolName := s.config["zfs.pool_name"]
	if zpoolName == "" {
		zpoolName = s.name
	}

	output, err := exec.Command(
		"zpool",
		"create", zpoolName, vdev,
		"-f", "-m", "none", "-O", "compression=on").CombinedOutput()
	if err != nil {
		return fmt.Errorf("Failed to create the ZFS pool: %s", output)
	}

	return nil
}

func (s *storagePool) zfsPoolDelete() error {
	zpoolName := s.config["zfs.pool_name"]
	if zpoolName == "" {
		zpoolName = s.name
	}

	// TODO: Check if any containers or images are using the pool.
	output, err := exec.Command(
		"zpool",
		"destroy", zpoolName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("Failed to delete the ZFS pool: %s", output)
	}

	// Cleanup storage
	vdev := s.config["source"]
	if filepath.IsAbs(vdev) && !shared.IsBlockdevPath(vdev) {
		os.RemoveAll(vdev)
	}

	return nil
}

// zfs functions.
func zfsPoolCheck(pool string) error {
	output, err := exec.Command(
		"zfs", "get", "type", "-H", "-o", "value", pool).CombinedOutput()
	if err != nil {
		return fmt.Errorf(strings.Split(string(output), "\n")[0])
	}

	poolType := strings.Split(string(output), "\n")[0]
	if poolType != "filesystem" {
		return fmt.Errorf("Unsupported pool type: %s", poolType)
	}

	return nil
}

func zfsPoolListSubvolumes(path string) ([]string, error) {
	output, err := exec.Command(
		"zfs",
		"list",
		"-t", "filesystem",
		"-o", "name",
		"-H",
		"-r", path).CombinedOutput()
	if err != nil {
		// s.log.Error("zfs list failed", log.Ctx{"output": string(output)})
		return []string{}, fmt.Errorf("Failed to list ZFS filesystems: %s", output)
	}

	children := []string{}
	for _, entry := range strings.Split(string(output), "\n") {
		if entry == "" {
			continue
		}

		if entry == path {
			continue
		}

		children = append(children, strings.TrimPrefix(entry, fmt.Sprintf("%s/", path)))
	}

	return children, nil
}
