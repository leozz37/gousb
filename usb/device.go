// Copyright 2013 Google Inc.  All rights reserved.
// Copyright 2016 the gousb Authors.  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package usb

import (
	"fmt"
	"sync"
)

// Device represents an opened USB device.
type Device struct {
	handle *libusbDevHandle

	// Embed the device information for easy access
	*Descriptor

	// Claimed config
	mu      sync.Mutex
	claimed *Config
}

// Reset performs a USB port reset to reinitialize a device.
func (d *Device) Reset() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.claimed != nil {
		return fmt.Errorf("can't reset a device with an open configuration")
	}
	return libusb.reset(d.handle)
}

// ActiveConfig returns the config id (not the index) of the active configuration.
// This corresponds to the ConfigInfo.Config field.
func (d *Device) ActiveConfig() (int, error) {
	ret, err := libusb.getConfig(d.handle)
	return int(ret), err
}

// Config returns a USB device set to use a particular config.
// The cfg provided is the config id (not the index) of the configuration to set,
// which corresponds to the ConfigInfo.Config field.
// USB supports only one active config per device at a time. Config claims the
// device before setting the desired config and keeps it locked until Close is called.
func (d *Device) Config(cfgNum int) (*Config, error) {
	cfg := &Config{
		dev:     d,
		claimed: make(map[int]bool),
	}
	var found bool
	for _, info := range d.Descriptor.Configs {
		if info.Config == cfgNum {
			found = true
			cfg.Info = info
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("configuration id %d not found in the descriptor of the device %s", cfg, d)
	}
	if err := libusb.setConfig(d.handle, uint8(cfgNum)); err != nil {
		return nil, fmt.Errorf("failed to set active config %d for the device %s: %v", cfgNum, d, err)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.claimed = cfg
	return cfg, nil
}

// Close closes the device.
func (d *Device) Close() error {
	if d.handle == nil {
		return fmt.Errorf("double close on device %s", d)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.claimed != nil {
		return fmt.Errorf("can't release the device %s, it has an open config %s", d, d.claimed.Info.Config)
	}
	libusb.close(d.handle)
	d.handle = nil
	return nil
}

// GetStringDescriptor returns a device string descriptor with the given index
// number. The first supported language is always used and the returned
// descriptor string is converted to ASCII (non-ASCII characters are replaced
// with "?").
func (d *Device) GetStringDescriptor(descIndex int) (string, error) {
	return libusb.getStringDesc(d.handle, descIndex)
}

// SetAutoDetach enables/disables libusb's automatic kernel driver detachment.
// When autodetach is enabled libusb will automatically detach the kernel driver
// on the interface and reattach it when releasing the interface.
// Automatic kernel driver detachment is disabled on newly opened device handles by default.
func (d *Device) SetAutoDetach(autodetach bool) error {
	var autodetachInt int
	switch autodetach {
	case true:
		autodetachInt = 1
	case false:
		autodetachInt = 0
	}
	return libusb.setAutoDetach(d.handle, autodetachInt)
}
