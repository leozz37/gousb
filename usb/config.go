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
	"time"
)

// ConfigInfo contains the information about a USB device configuration.
type ConfigInfo struct {
	// Config is the configuration number.
	Config int
	// SelfPowered is true if the device is powered externally, i.e. not
	// drawing power from the USB bus.
	SelfPowered bool
	// RemoteWakeup is true if the device supports remote wakeup.
	RemoteWakeup bool
	// MaxPower is the maximum current the device draws from the USB bus
	// in this configuration.
	MaxPower Milliamperes
	// Interfaces has a list of USB interfaces available in this configuration.
	Interfaces []InterfaceInfo
}

// String returns the human-readable description of the configuration.
func (c ConfigInfo) String() string {
	return fmt.Sprintf("config=%d", c.Config)
}

// Config represents a USB device set to use a particular configuration.
// Only one Config of a particular device can be used at any one time.
type Config struct {
	Info           ConfigInfo
	ControlTimeout time.Duration

	dev *Device

	// Claimed interfaces
	mu      sync.Mutex
	claimed map[int]bool
}

// Close releases the underlying device, allowing the caller to switch the device to a different configuration.
func (c *Config) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.claimed) > 0 {
		var ifs []int
		for k := range c.claimed {
			ifs = append(ifs, k)
		}
		return fmt.Errorf("failed to release %s, interfaces %v are still open", c, ifs)
	}
	c.dev.mu.Lock()
	defer c.dev.mu.Unlock()
	c.dev.claimed = nil
	return nil
}

func (c *Config) String() string {
	return fmt.Sprintf("%s,%s", c.dev.String(), c.Info.String())
}

// Control sends a control request to the device.
func (c *Config) Control(rType, request uint8, val, idx uint16, data []byte) (int, error) {
	return libusb.control(c.dev.handle, c.ControlTimeout, rType, request, val, idx, data)
}

// Interface claims and returns an interface on a USB device.
func (c *Config) Interface(intf, alt int) (*Interface, error) {
	if intf < 0 || intf >= len(c.Info.Interfaces) {
		return nil, fmt.Errorf("interface %d not found in %s. Interface number needs to be a 0-based index into the interface table, which has %d elements.", intf, c, len(c.Info.Interfaces))
	}
	ifInfo := c.Info.Interfaces[intf]
	if alt < 0 || alt >= len(ifInfo.AltSettings) {
		return nil, fmt.Errorf("Inteface %d does not have alternate setting %d. Alt setting needs to be a 0-based index into the settings table, which has %d elements.", ifInfo, alt, len(ifInfo.AltSettings))
	}

	// Claim the interface
	if err := libusb.claim(c.dev.handle, uint8(intf)); err != nil {
		return nil, fmt.Errorf("failed to claim interface %d on %s: %v", intf, c, err)
	}

	if err := libusb.setAlt(c.dev.handle, uint8(intf), uint8(alt)); err != nil {
		libusb.release(c.dev.handle, uint8(intf))
		return nil, fmt.Errorf("failed to set alternate config %d on interface %d of %s: %v", alt, intf, c, err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.claimed[intf] = true
	return &Interface{
		Setting: ifInfo.AltSettings[alt],
		config:  c,
	}, nil
}
