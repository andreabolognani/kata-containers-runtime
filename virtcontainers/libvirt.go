// Copyright (c) 2020 Red Hat, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//

package virtcontainers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kata-containers/runtime/virtcontainers/device/config"
	persistapi "github.com/kata-containers/runtime/virtcontainers/persist/api"
	"github.com/kata-containers/runtime/virtcontainers/pkg/uuid"
	"github.com/kata-containers/runtime/virtcontainers/types"
	"github.com/kata-containers/runtime/virtcontainers/utils"
	"github.com/sirupsen/logrus"
	virt "libvirt.org/libvirt-go"
	virtxml "libvirt.org/libvirt-go-xml"
)

const (
	libvirtDefaultURI    = "qemu:///system"
	libvirtConsoleSocket = "console.sock"
)

var libvirtDefaultKernelParams = []Param{
	{"quiet", ""},
	{"tsc", "reliable"},
	{"no_timer_check", ""},
	{"rcupdate.rcu_expedited", "1"},
	{"i8042.direct", "1"},
	{"i8042.dumbkbd", "1"},
	{"i8042.nopnp", "1"},
	{"i8042.noaux", "1"},
	{"noreplace-smp", ""},
	{"reboot", "k"},
	{"console", "hvc0"},
	{"console", "hvc1"},
	{"iommu", "off"},
	{"cryptomgr.notests", ""},
	{"net.ifnames", "0"},
	{"pci", "lastbus=0"},
	{"panic", "1"},
}

type libvirt struct {
	id             string
	store          persistapi.PersistDriver
	config         *HypervisorConfig
	libvirtUUID    string
	libvirtRoot    string
	libvirtURI     string
	libvirtConfig  *virtxml.Domain
	libvirtConnect *virt.Connect
	libvirtDomain  *virt.Domain
}

func (v *libvirt) logger() *logrus.Entry {
	return virtLog.WithField("subsystem", "libvirt")
}

func (v *libvirt) funcLogger(funcName string) *logrus.Entry {
	return v.logger().WithField("func", funcName)
}

func (v *libvirt) capabilities() types.Capabilities {
	v.logger().Info("capabilities() called")

	var caps types.Capabilities

	caps.SetFsSharingSupport()

	return caps
}

func (v *libvirt) hypervisorConfig() HypervisorConfig {
	v.logger().Info("hypervisorConfig() called")
	return *v.config
}

func (v *libvirt) initLibvirtConnect() error {
	l := v.funcLogger("initLibvirtConnect")
	l.Debug()

	if v.libvirtConnect != nil {
		l.Debug("connect already exists")
		return nil
	}

	err := virt.EventRegisterDefaultImpl()
	if err != nil {
		return err
	}

	l.Debug("event loop registered")

	go func() {
		for {
			err := virt.EventRunDefaultImpl()
			if err != nil {
				panic(err)
			}
		}
	}()

	l.Debug("event loop running")

	v.libvirtConnect, err = virt.NewConnect(v.libvirtURI)
	if err != nil {
		return err
	}

	l.Debug("connected")

	return nil
}

func (v *libvirt) initLibvirtDomain() error {
	l := v.funcLogger("initLibvirtDomain")
	l.Debug()

	if v.libvirtDomain != nil {
		l.Debug("domain already exists")
		return nil
	}

	var err error
	v.libvirtDomain, err = v.libvirtConnect.LookupDomainByName(v.libvirtConfig.Name)
	if err != nil {
		return err
	}

	l.Debug("domain found")

	return nil
}

func (v *libvirt) initLibvirt() error {
	l := v.funcLogger("initLibvirt")
	l.Debug()

	err := v.initLibvirtConnect()
	if err != nil {
		return err
	}

	err = v.initLibvirtDomain()
	if err != nil {
		return err
	}

	return nil
}

func (v *libvirt) prepareHostFilesystem() error {
	l := v.funcLogger("prepareHostFilesystem")
	l.Debug()

	libvirtConfDir := filepath.Join(v.libvirtRoot, "etc")

	paths := []string{
		filepath.Join(v.store.RunStoragePath(), v.id),
		filepath.Join(v.store.RunVMStoragePath(), v.id),
		v.libvirtRoot,
		libvirtConfDir,
	}

	for _, path := range paths {
		err := os.MkdirAll(path, os.FileMode(0755)|os.ModeDir)
		if err != nil {
			return err
		}

		l.WithField("path", path).Debug("host directory created")
	}

	lnTarget := v.libvirtRoot
	lnName := filepath.Join(v.store.RunVMStoragePath(), v.id, "libvirt")
	err := os.Symlink(lnTarget, lnName)
	if err != nil {
		return err
	}

	l.WithField("lnTarget", lnTarget).WithField("lnName", lnName).Debug("symlink created")

	qemuConf, err := os.Create(filepath.Join(libvirtConfDir, "qemu.conf"))
	if err != nil {
		return err
	}
	defer qemuConf.Close()

	qemuConf.WriteString("stdio_handler = \"file\"\n")

	return nil
}

func uuidRemoveDashes(uuid string) string {
	chunks := []string{
		uuid[0:8],
		uuid[9:13],
		uuid[14:18],
		uuid[19:23],
		uuid[24:],
	}
	return strings.Join(chunks, "")
}

func uuidAddDashes(uuid string) string {
	chunks := []string{
		uuid[0:8],
		uuid[8:12],
		uuid[12:16],
		uuid[16:20],
		uuid[20:],
	}
	return strings.Join(chunks, "-")
}

func (v *libvirt) createSandbox(ctx context.Context, id string, networkNS NetworkNamespace, hypervisorConfig *HypervisorConfig, stateful bool) error {
	l := v.funcLogger("createSandbox")
	l.WithField("ctx", ctx).WithField("id", id).WithField("networkNS", networkNS).WithField("hypervisorConfig", hypervisorConfig).WithField("stateful", stateful).Debug()

	v.id = id
	v.config = hypervisorConfig

	err := v.config.valid()
	if err != nil {
		return err
	}

	// If this symlink exists, it will point to the libvirtRoot we have
	// created earlier; it not existing is not an error
	rootLink := filepath.Join(v.store.RunVMStoragePath(), v.id, "libvirt")
	_, err = os.Stat(rootLink)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if err == nil {
		// The symlink exists: it will point to the previously created
		// libvirtRoot, whose last element is the libvirtUUID
		v.libvirtRoot, err = os.Readlink(rootLink)
		if err != nil {
			return err
		}
		v.libvirtUUID = uuidAddDashes(filepath.Base(v.libvirtRoot))
	} else {
		// The symlink doesn't exist: generate a fresh libvirtUUID and
		// derive libvirtRoot from it
		v.libvirtUUID = uuid.Generate().String()
		v.libvirtRoot = filepath.Join(v.store.RunVMStoragePath(), "..", "libvirt", uuidRemoveDashes(v.libvirtUUID))
	}

	v.libvirtURI = fmt.Sprintf("qemu:///embed?root=%s", v.libvirtRoot)

	consolePath, err := v.getSandboxConsole(id)
	if err != nil {
		return err
	}

	kernelParams := libvirtDefaultKernelParams
	kernelParams = append(kernelParams, Param{"nr_cpus", fmt.Sprintf("%d", v.config.DefaultMaxVCPUs)})
	kernelParams = append(kernelParams, Param{"agent.use_vsock", "false"})
	kernelParams = append(kernelParams, v.config.KernelParams...)

	kernelCmdline := strings.Join(SerializeParams(kernelParams, "="), " ")

	v.libvirtConfig = &virtxml.Domain{
		Type: "kvm",
		UUID: v.libvirtUUID,
		Name: "sandbox",
		VCPU: &virtxml.DomainVCPU{
			Current: fmt.Sprintf("%d", v.config.NumVCPUs),
			Value:   int(v.config.DefaultMaxVCPUs),
		},
		Memory: &virtxml.DomainMemory{
			Unit:  "MiB",
			Value: uint(v.config.MemorySize),
		},
		OS: &virtxml.DomainOS{
			Type: &virtxml.DomainOSType{
				Type:    "hvm",
				Machine: v.config.HypervisorMachineType,
			},
			Kernel:  v.config.KernelPath,
			Initrd:  v.config.InitrdPath,
			Cmdline: kernelCmdline,
		},
		Features: &virtxml.DomainFeatureList{
			ACPI: &virtxml.DomainFeature{},
			APIC: &virtxml.DomainFeatureAPIC{},
			IOAPIC: &virtxml.DomainFeatureIOAPIC{
				Driver: "kvm",
			},
			PMU: &virtxml.DomainFeatureState{
				State: "off",
			},
		},
		CPU: &virtxml.DomainCPU{
			Mode: "host-passthrough",
		},
		Clock: &virtxml.DomainClock{
			Timer: []virtxml.DomainTimer{
				virtxml.DomainTimer{
					Name:       "pit",
					TickPolicy: "discard",
				},
			},
		},
		Devices: &virtxml.DomainDeviceList{
			Emulator: v.config.HypervisorPath,
			Consoles: []virtxml.DomainConsole{
				virtxml.DomainConsole{
					Source: &virtxml.DomainChardevSource{
						UNIX: &virtxml.DomainChardevSourceUNIX{
							Mode: "bind",
							Path: consolePath,
						},
					},
					Target: &virtxml.DomainConsoleTarget{
						Type: "virtio",
					},
				},
			},
			Controllers: []virtxml.DomainController{
				virtxml.DomainController{
					Type:  "usb",
					Model: "none",
				},
			},
			MemBalloon: &virtxml.DomainMemBalloon{
				Model: "none",
			},
			RNGs: []virtxml.DomainRNG{
				virtxml.DomainRNG{
					Model: "virtio",
					Backend: &virtxml.DomainRNGBackend{
						Random: &virtxml.DomainRNGBackendRandom{
							Device: "/dev/urandom",
						},
					},
				},
			},
			Channels:    []virtxml.DomainChannel{},
			Filesystems: []virtxml.DomainFilesystem{},
			Interfaces:  []virtxml.DomainInterface{},
		},
	}

	if v.config.SharedFS == config.VirtioFS {
		v.libvirtConfig.MemoryBacking = &virtxml.DomainMemoryBacking{
			MemoryAccess: &virtxml.DomainMemoryAccess{
				Mode: "shared",
			},
		}
		cellId := uint(0)
		v.libvirtConfig.CPU.Numa = &virtxml.DomainNuma{
			Cell: []virtxml.DomainCell{
				virtxml.DomainCell{
					ID:        &cellId,
					CPUs:      fmt.Sprintf("0-%d", v.config.DefaultMaxVCPUs-1),
					Memory:    fmt.Sprintf("%d", v.config.MemorySize),
					Unit:      "MiB",
					MemAccess: "shared",
				},
			},
		}
	}

	return nil
}

func (v *libvirt) startSandbox(timeout int) error {
	l := v.funcLogger("startSandbox")
	l.WithField("timeout", timeout).Debug()

	err := v.prepareHostFilesystem()
	if err != nil {
		return err
	}

	domXML, err := v.libvirtConfig.Marshal()
	if err != nil {
		return err
	}

	l.WithField("domXML", domXML).Debug()

	err = v.initLibvirtConnect()
	if err != nil {
		return err
	}

	dom, err := v.libvirtConnect.DomainDefineXML(domXML)
	if err != nil {
		return err
	}
	defer dom.Free()

	l.Debug("domain defined")

	err = dom.Create()
	if err != nil {
		return err
	}

	l.Debug("domain created")

	return nil
}

func (v *libvirt) stopSandbox() error {
	l := v.funcLogger("stopSandbox")
	l.Debug()

	err := v.initLibvirt()
	if err != nil {
		return err
	}

	err = v.libvirtDomain.Destroy()
	if err == nil {
		l.Debug("domain destroyed")
	} else {
		l.Debug("failed to destroy domain")
	}

	err = v.libvirtDomain.Undefine()
	if err != nil {
		return err
	}

	l.Debug("domain undefined")

	return nil
}

func (v *libvirt) pauseSandbox() error {
	v.logger().Info("pauseSandbox() called")
	return errors.New("pauseSandbox() failed")
}

func (v *libvirt) resumeSandbox() error {
	v.logger().Info("resumeSandbox() called")
	return errors.New("resumeSandbox() failed")
}

func (v *libvirt) saveSandbox() error {
	v.logger().Info("saveSandbox() called")
	return errors.New("saveSandbox() failed")
}

func (v *libvirt) addDevice(devInfo interface{}, devType deviceType) error {
	l := v.funcLogger("addDevice")
	l.WithField("devInfo", devInfo).WithField("devType", devType).Debug()

	switch dev := devInfo.(type) {
	case types.Socket:
		sock := &virtxml.DomainChannel{
			Source: &virtxml.DomainChardevSource{
				UNIX: &virtxml.DomainChardevSourceUNIX{
					Mode: "bind",
					Path: dev.HostPath,
				},
			},
			Target: &virtxml.DomainChannelTarget{
				VirtIO: &virtxml.DomainChannelTargetVirtIO{
					Name: dev.Name,
				},
			},
		}
		v.libvirtConfig.Devices.Channels = append(v.libvirtConfig.Devices.Channels, *sock)
	case types.Volume:
		fs := &virtxml.DomainFilesystem{
			Source: &virtxml.DomainFilesystemSource{
				Mount: &virtxml.DomainFilesystemSourceMount{
					Dir: dev.HostPath,
				},
			},
			Target: &virtxml.DomainFilesystemTarget{
				Dir: dev.MountTag,
			},
		}
		if v.config.SharedFS == config.VirtioFS {
			fs.Driver = &virtxml.DomainFilesystemDriver{
				Type: "virtiofs",
			}
		}
		v.libvirtConfig.Devices.Filesystems = append(v.libvirtConfig.Devices.Filesystems, *fs)
	case Endpoint:
		l.WithField("type", dev.Type()).Debug()

		iface := &virtxml.DomainInterface{
			Source: &virtxml.DomainInterfaceSource{
				Ethernet: &virtxml.DomainInterfaceSourceEthernet{},
			},
			Target: &virtxml.DomainInterfaceTarget{
				Dev:     dev.NetworkPair().TapInterface.TAPIface.Name,
				Managed: "no",
			},
			Model: &virtxml.DomainInterfaceModel{
				Type: "virtio",
			},
			MAC: &virtxml.DomainInterfaceMAC{
				Address: dev.HardwareAddr(),
			},
		}
		v.libvirtConfig.Devices.Interfaces = append(v.libvirtConfig.Devices.Interfaces, *iface)
	default:
		break
	}

	return nil
}

func (v *libvirt) hotplugAddDevice(devInfo interface{}, devType deviceType) (interface{}, error) {
	v.logger().Info("hotplugAddDevice() called")
	return nil, errors.New("hotplugAddDevice() failed")
}

func (v *libvirt) hotplugRemoveDevice(devInfo interface{}, devType deviceType) (interface{}, error) {
	v.logger().Info("hotplugRemoveDevice() called")
	return nil, errors.New("hotplugRemoveDevice() failed")
}

func (v *libvirt) getSandboxConsole(id string) (string, error) {
	l := v.funcLogger("getSandboxConsole")
	l.WithField("id", id).Debug()

	return utils.BuildSocketPath(v.store.RunVMStoragePath(), id, libvirtConsoleSocket)
}

func (v *libvirt) resizeMemory(reqMemMB uint32, memoryBlockSizeMB uint32, probe bool) (uint32, memoryDevice, error) {
	l := v.funcLogger("resizeMemory")
	l.WithField("reqMemMB", reqMemMB).WithField("memoryBlockSizeMB", memoryBlockSizeMB).WithField("probe", probe).Debug()

	return 0, memoryDevice{}, errors.New("resizeMemory() failed")
}
func (v *libvirt) resizeVCPUs(reqVCPUs uint32) (uint32, uint32, error) {
	l := v.funcLogger("resizeVCPUs")
	l.WithField("reqVCPUs", reqVCPUs).Debug()

	maxVCPUs := uint32(v.libvirtConfig.VCPU.Value)

	if reqVCPUs > maxVCPUs {
		// Can't go beyond the max
		l.WithField("reqVCPUs", reqVCPUs).WithField("maxVCPUs", maxVCPUs).Warn("Capped vCPUs")
		reqVCPUs = maxVCPUs
	}

	err := v.initLibvirt()
	if err != nil {
		return 0, 0, err
	}

	tmp, err := v.libvirtDomain.GetVcpusFlags(virt.DOMAIN_VCPU_LIVE)
	if err != nil {
		return 0, 0, err
	}
	// Negative values are only returned for errors
	oldVCPUs := uint32(tmp)

	if oldVCPUs == reqVCPUs {
		// Nothing to do
		return oldVCPUs, oldVCPUs, nil
	}

	err = v.libvirtDomain.SetVcpusFlags(uint(reqVCPUs), virt.DOMAIN_VCPU_LIVE)
	if err != nil {
		return 0, 0, err
	}

	/*
		for {
			tmp, err := v.libvirtDomain.GetVcpusFlags(virt.DOMAIN_VCPU_LIVE)
			if err != nil {
				return 0, 0, err
			}
			currentVCPUs := uint32(tmp)
			if currentVCPUs == reqVCPUs {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
	*/

	return oldVCPUs, reqVCPUs, nil
}

func (v *libvirt) disconnect() {
	v.logger().Info("disconnect() called")
}

func (v *libvirt) getThreadIDs() (vcpuThreadIDs, error) {
	v.logger().Info("getThreadIDs() called")
	return vcpuThreadIDs{}, errors.New("getThreadIDs() failed")
}

func (v *libvirt) cleanup() error {
	v.logger().Info("cleanup() called")
	return errors.New("cleanup() failed")
}

func (v *libvirt) getPids() []int {
	v.logger().Info("getPids() called")
	return nil
}

func (v *libvirt) fromGrpc(ctx context.Context, hypervisorConfig *HypervisorConfig, j []byte) error {
	v.logger().Info("fromGrpc() called")
	return errors.New("fromGrpc() failed")
}

func (v *libvirt) toGrpc() ([]byte, error) {
	v.logger().Info("toGrpc() called")
	return nil, errors.New("toGrpc() failed")
}

func (v *libvirt) save() (s persistapi.HypervisorState) {
	v.logger().Info("save() called")
	return
}

func (v *libvirt) load(s persistapi.HypervisorState) {
	v.logger().Info("load() called")
	return
}

func (v *libvirt) check() error {
	v.logger().Info("check() called")
	return errors.New("check() failed")
}

func (v *libvirt) generateSocket(id string, useVsock bool) (interface{}, error) {
	l := v.funcLogger("generateSocket")
	l.WithField("id", id).WithField("useVsock", useVsock).Debug()

	sock, err := generateVMSocket(id, useVsock, v.store.RunVMStoragePath())
	if err == nil {
		l.WithField("sock", sock).Debug()
	}

	return sock, err
}

// vim: set noexpandtab :