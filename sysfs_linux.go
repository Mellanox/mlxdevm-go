package mlxdevm

// The following operations are currently not supported by the 'mlxdevm' devlink:
// - Retrieving the auxiliary device name of an SF port, e.g., mlx5_core.sf.3.
// - Unbinding the SF port from the default configuration driver and binding it to the actual SF driver.
// These operations must be performed through sysfs. While we are looking for
// adding devlink support for them in the long term, these functions within this
// file can be used to accomplish the task until that support is implemented.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	utilfs "github.com/Mellanox/sriovnet/pkg/utils/filesystem"
)

const (
	PcidevPrefix       = "device"
	NetSysDir          = "/sys/class/net"
	AuxDevDir          = "/sys/bus/auxiliary/devices"
	MlxCoreSfCfgUnbind = "/sys/bus/auxiliary/drivers/mlx5_core.sf_cfg/unbind"
	MlxCoreSfBind      = "/sys/bus/auxiliary/drivers/mlx5_core.sf/bind"
)

// Get the auxiliary device name of a SF by 'sfnum'.
// Each SF has a unique 'sfnum>'.
// Note: SF's state must be set as active before calling this function.
func GetSFAuxDev(sfnum uint32) (string, error) {
	devices, err := utilfs.Fs.ReadDir(AuxDevDir)
	if err != nil {
		return "", err
	}
	for _, device := range devices {
		sfNumFile := filepath.Join(AuxDevDir, device.Name(), "sfnum")
		devSFNum, err := utilfs.Fs.ReadFile(sfNumFile)
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(devSFNum)) == strconv.FormatUint(uint64(sfnum), 10) {
			return device.Name(), nil
		}
	}
	return "", fmt.Errorf("can not find aux dev for SF %d", sfnum)
}

// This function unbinds the SF from the default config driver and bind it to
// the actual SF driver -- SF's opstate is changed from 'detached' to 'attached'.
// Note: SF must be properly configured and its state has been set as active
//
//	before calling this function
func DeploySF(auxDev string) error {
	// equivalent to:
	// echo mlx5_core.sf.<serial> > /sys/bus/auxiliary/drivers/mlx5_core.sf_cfg/unbind
	err := utilfs.Fs.WriteFile(MlxCoreSfCfgUnbind, []byte(auxDev), os.FileMode(0200))
	if err != nil {
		return fmt.Errorf("fail to unbind SF %s from default config driver: %v", auxDev, err)
	}

	// equivalent to:
	// echo mlx5_core.sf.<serial> > /sys/bus/auxiliary/drivers/mlx5_core.sf/bind
	err = utilfs.Fs.WriteFile(MlxCoreSfBind, []byte(auxDev), os.FileMode(0200))
	if err != nil {
		return fmt.Errorf("fail to bind SF %s to SF driver: %v", auxDev, err)
	}

	return nil
}
