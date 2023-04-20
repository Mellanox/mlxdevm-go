# mlxdevm-go
mlxdevm library for for device management in go language

## Overview
Subfunction(SF) aka scalable function are managed by devlink interface in upstream kernel.
They can be also managed in Mellanox OFED distrubtion using a similar interface called mlxdevm.
This is helpful to use SFs in older kernel distributions where devlink interface of latest and greast kernel is not unavailable.

Only difference between the two interfaces are their socket name.
For example devlink socket name is "devlink" vs mlxdevm socket name is "mlxdevm".

Container orchestration software such as CNI or device plugin which needs to operate over both the interfaces (upstream devlink and mlxdevm) in simple and elegant way.

This package enables orchestration software to use upstream devlink compliant APIs over mlxdevm interface.

## How to use this library in the application?

$ go get github.com/Mellanox/mlxdevm-go

Sample application:

```go
package main

import (
    mlxdevm "github.com/Mellanox/mlxdevm-go"
    "fmt"
)

func main() {
    var portAttr mlxdevm.DevlinkPortAddAttrs
    
    portAttr.PfNumber = 0
    
    portAttr.SfNumber = 88 // Any number starting 0 to 999
    portAttr.SfNumberValid = true
    // To use upstream devlink interface
    dl_port, err := mlxdevm.DevlinkPortAdd("devlink", "pci", "0000:06:00.0", mlxdevm.DEVLINK_PORT_FLAVOUR_PCI_SF, portAttr)
    if err != nil {
        return
    }
    fmt.Printf("Port = ", dl_port)
    
    portAttr.SfNumber = 99 // Any number starting 0 to 999
    // To use mlxdevm interface
    dl_port2, err2 := mlxdevm.DevlinkPortAdd("mlxdevm", "pci", "0000:06:00.0", mlxdevm.DEVLINK_PORT_FLAVOUR_PCI_SF, portAttr)
    if err2 != nil {
        return
    }
    fmt.Printf("Port = ", dl_port2)
}
```
