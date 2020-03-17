// Copyright (c) 2020 Red Hat, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//

package virtcontainers

import (
	"context"
	"errors"

	persistapi "github.com/kata-containers/runtime/virtcontainers/persist/api"
	"github.com/kata-containers/runtime/virtcontainers/types"
	"github.com/sirupsen/logrus"
)

type libvirt struct {
	id     string
	store  persistapi.PersistDriver
	config *HypervisorConfig
}

func (v *libvirt) logger() *logrus.Entry {
	return virtLog.WithField("subsystem", "libvirt")
}

func (v *libvirt) funcLogger(funcName string) *logrus.Entry {
	return v.logger().WithField("func", funcName)
}

func (v *libvirt) capabilities() types.Capabilities {
	v.logger().Info("capabilities() called")
	return types.Capabilities{}
}

func (v *libvirt) hypervisorConfig() HypervisorConfig {
	v.logger().Info("hypervisorConfig() called")
	return *v.config
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

	return nil
}

func (v *libvirt) startSandbox(timeout int) error {
	l := v.funcLogger("startSandbox")
	l.WithField("timeout", timeout).Debug()

	return errors.New("startSandbox() failed")
}

func (v *libvirt) stopSandbox() error {
	l := v.funcLogger("stopSandbox")
	l.Debug()

	return errors.New("stopSandbox() failed")
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

	return errors.New("addDevice() failed")
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

	return "", errors.New("getSandboxConsole() failed")
}

func (v *libvirt) resizeMemory(reqMemMB uint32, memoryBlockSizeMB uint32, probe bool) (uint32, memoryDevice, error) {
	l := v.funcLogger("resizeMemory")
	l.WithField("reqMemMB", reqMemMB).WithField("memoryBlockSizeMB", memoryBlockSizeMB).WithField("probe", probe).Debug()

	return 0, memoryDevice{}, errors.New("resizeMemory() failed")
}
func (v *libvirt) resizeVCPUs(reqVCPUs uint32) (uint32, uint32, error) {
	l := v.funcLogger("resizeVCPUs")
	l.WithField("reqVCPUs", reqVCPUs).Debug()

	return 0, 0, errors.New("resizeVCPUs() failed")
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

	return nil, errors.New("generateSocket() failed")
}

// vim: set noexpandtab :
