package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/liuscraft/orion-x/internal/logging"
	"github.com/liuscraft/orion-x/internal/tools"
	"github.com/liuscraft/orion-x/cmd/wsserver/wsproto"
)

// iotProperty mirrors wsproto.IoTProperty but stores a live value.
type iotProperty struct {
	Description string
	Type        string
	Value       any
}

// iotDevice holds the descriptor and current state for one IoT device.
type iotDevice struct {
	Name        string
	Description string
	Properties  map[string]iotProperty
	Methods     map[string]wsproto.IoTMethod
}

// iotManager manages per-connection IoT device state. It holds a reference to
// the per-connection tools.Registry and registers specs directly when
// descriptors arrive.
type iotManager struct {
	conn     safeWriter
	registry *tools.Registry
	devices  map[string]*iotDevice
}

type safeWriter interface {
	WriteJSON(v any) error
}

func newIoTManager(conn safeWriter, registry *tools.Registry) *iotManager {
	return &iotManager{
		conn:     conn,
		registry: registry,
		devices:  make(map[string]*iotDevice),
	}
}

// handleDescriptors registers device capability declarations and adds the
// corresponding tools.Spec entries to the per-connection registry.
func (m *iotManager) handleDescriptors(descriptors []wsproto.IoTDescriptor) {
	for _, d := range descriptors {
		dev := &iotDevice{
			Name:        d.Name,
			Description: d.Description,
			Properties:  make(map[string]iotProperty),
			Methods:     make(map[string]wsproto.IoTMethod),
		}
		for k, p := range d.Properties {
			var zero any
			switch p.Type {
			case "number":
				zero = float64(0)
			case "boolean":
				zero = false
			default:
				zero = ""
			}
			dev.Properties[k] = iotProperty{Description: p.Description, Type: p.Type, Value: zero}
		}
		for k, method := range d.Methods {
			dev.Methods[k] = method
		}
		m.devices[d.Name] = dev
		logging.Infof("wsserver/iot: registered device %q (%d properties, %d methods)",
			d.Name, len(dev.Properties), len(dev.Methods))

		for _, spec := range m.specsForDevice(dev) {
			m.registry.Add(spec)
		}
	}
}

// handleStates updates live property values from a client state report.
func (m *iotManager) handleStates(states []wsproto.IoTState) {
	for _, s := range states {
		dev, ok := m.devices[s.Name]
		if !ok {
			continue
		}
		for k, v := range s.State {
			if prop, ok := dev.Properties[k]; ok {
				prop.Value = v
				dev.Properties[k] = prop
				logging.Infof("wsserver/iot: %s.%s = %v", s.Name, k, v)
			}
		}
	}
}

func (m *iotManager) specsForDevice(dev *iotDevice) []tools.Spec {
	var specs []tools.Spec

	// Query tool per property: get_<device>_<property>
	for propName, prop := range dev.Properties {
		name := fmt.Sprintf("get_%s_%s", strings.ToLower(dev.Name), strings.ToLower(propName))
		desc := fmt.Sprintf("查询%s的%s", dev.Description, prop.Description)
		devName := dev.Name
		pName := propName
		mgr := m

		specs = append(specs, tools.Spec{
			Name:        name,
			Description: desc,
			Parameters: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
			Execute: func(ctx context.Context, _ json.RawMessage) (tools.Result, error) {
				d, ok := mgr.devices[devName]
				if !ok {
					return tools.Result{Error: fmt.Errorf("device %s not found", devName)}, nil
				}
				p, ok := d.Properties[pName]
				if !ok {
					return tools.Result{Error: fmt.Errorf("property %s not found", pName)}, nil
				}
				out, _ := json.Marshal(p.Value)
				return tools.Result{Output: string(out)}, nil
			},
		})
	}

	// Control tool per method: <device>_<method>
	for methodName, method := range dev.Methods {
		name := fmt.Sprintf("%s_%s", strings.ToLower(dev.Name), strings.ToLower(methodName))
		desc := fmt.Sprintf("%s - %s", dev.Description, method.Description)
		devName := dev.Name
		mName := methodName
		mgr := m

		props := make(map[string]any, len(method.Parameters))
		required := make([]string, 0, len(method.Parameters))
		for pn, pi := range method.Parameters {
			props[pn] = map[string]any{"type": pi.Type, "description": pi.Description}
			required = append(required, pn)
		}

		specs = append(specs, tools.Spec{
			Name:        name,
			Description: desc,
			Parameters: map[string]any{
				"type":       "object",
				"properties": props,
				"required":   required,
			},
			Execute: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
				var params map[string]any
				if len(args) > 0 {
					_ = json.Unmarshal(args, &params)
				}
				if err := mgr.sendCommand(ctx, devName, mName, params); err != nil {
					return tools.Result{Error: err}, nil
				}
				return tools.Result{Output: "ok"}, nil
			},
		})
	}

	return specs
}

func (m *iotManager) sendCommand(_ context.Context, device, method string, params map[string]any) error {
	cmd := wsproto.IoTCommand{Name: device, Method: method}
	if len(params) > 0 {
		cmd.Parameters = params
	}
	msg := wsproto.NewIoTCommandMessage("", []wsproto.IoTCommand{cmd})
	return m.conn.WriteJSON(msg)
}
