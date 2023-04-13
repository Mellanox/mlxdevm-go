package main

import (
    mlxdevm "github.com/Mellanox/mlxdevm-go"
    "fmt"
)

func main() {
    var portAttr mlxdevm.DevLinkPortAddAttrs

    portAttr.PfNumber = 0
    portAttr.SfNumberValid = true
    portAttr.SfNumber = 99 // Any number starting 0 to 999
    // To use mlxdevm interface
    dl_port2, err2 := mlxdevm.DevLinkPortAdd("mlxdevm", "pci", "0000:06:00.0", mlxdevm.DEVLINK_PORT_FLAVOUR_PCI_SF, portAttr)
    if err2 != nil {
        return
    }
    fmt.Printf("Port = %v", dl_port2)
}
