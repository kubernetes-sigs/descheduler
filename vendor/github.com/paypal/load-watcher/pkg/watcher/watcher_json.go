/*
Copyright 2020 PayPal

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Generated from gojay generator tool, with some bug fixes

package watcher

import (
	"github.com/francoispqt/gojay"
)

type Metrices []Metric

func (s *Metrices) UnmarshalJSONArray(dec *gojay.Decoder) error {
	var value = Metric{}
	if err := dec.Object(&value); err != nil {
		return err
	}
	*s = append(*s, value)
	return nil
}

func (s Metrices) MarshalJSONArray(enc *gojay.Encoder) {
	for i := range s {
		enc.Object(&s[i])
	}
}

func (s Metrices) IsNil() bool {
	return len(s) == 0
}

// MarshalJSONObject implements MarshalerJSONObject
func (d *Data) MarshalJSONObject(enc *gojay.Encoder) {
	enc.ObjectKey("NodeMetricsMap", &d.NodeMetricsMap)
}

// IsNil checks if instance is nil
func (d *Data) IsNil() bool {
	return d == nil
}

// UnmarshalJSONObject implements gojay's UnmarshalerJSONObject
func (d *Data) UnmarshalJSONObject(dec *gojay.Decoder, k string) error {

	switch k {
	case "NodeMetricsMap":
		err := dec.Object(&d.NodeMetricsMap)
		return err
	}
	return nil
}

// NKeys returns the number of keys to unmarshal
func (d *Data) NKeys() int { return 1 }

// MarshalJSONObject implements MarshalerJSONObject
func (m *Metadata) MarshalJSONObject(enc *gojay.Encoder) {
	enc.StringKey("dataCenter", m.DataCenter)
}

// IsNil checks if instance is nil
func (m *Metadata) IsNil() bool {
	return m == nil
}

// UnmarshalJSONObject implements gojay's UnmarshalerJSONObject
func (m *Metadata) UnmarshalJSONObject(dec *gojay.Decoder, k string) error {

	switch k {
	case "dataCenter":
		return dec.String(&m.DataCenter)

	}
	return nil
}

// NKeys returns the number of keys to unmarshal
func (m *Metadata) NKeys() int { return 1 }

// MarshalJSONObject implements MarshalerJSONObject
func (m *Metric) MarshalJSONObject(enc *gojay.Encoder) {
	enc.StringKey("name", m.Name)
	enc.StringKey("type", m.Type)
	enc.StringKey("operator", m.Operator)
	enc.StringKey("rollup", m.Rollup)
	enc.Float64Key("value", m.Value)
}

// IsNil checks if instance is nil
func (m *Metric) IsNil() bool {
	return m == nil
}

// UnmarshalJSONObject implements gojay's UnmarshalerJSONObject
func (m *Metric) UnmarshalJSONObject(dec *gojay.Decoder, k string) error {

	switch k {
	case "name":
		return dec.String(&m.Name)

	case "type":
		return dec.String(&m.Type)

	case "operator":
		return dec.String(&m.Operator)

	case "rollup":
		return dec.String(&m.Rollup)

	case "value":
		return dec.Float64(&m.Value)

	}
	return nil
}

// NKeys returns the number of keys to unmarshal
func (m *Metric) NKeys() int { return 5 }

// MarshalJSONObject implements MarshalerJSONObject
func (m *NodeMetrics) MarshalJSONObject(enc *gojay.Encoder) {
	var metricsSlice = Metrices(m.Metrics)
	enc.ArrayKey("metrics", metricsSlice)
	enc.ObjectKey("tags", &m.Tags)
	enc.ObjectKey("metadata", &m.Metadata)
}

// IsNil checks if instance is nil
func (m *NodeMetrics) IsNil() bool {
	return m == nil
}

// UnmarshalJSONObject implements gojay's UnmarshalerJSONObject
func (m *NodeMetrics) UnmarshalJSONObject(dec *gojay.Decoder, k string) error {

	switch k {
	case "metrics":
		var aSlice = Metrices{}
		err := dec.Array(&aSlice)
		if err == nil && len(aSlice) > 0 {
			m.Metrics = []Metric(aSlice)
		}
		return err

	case "tags":
		err := dec.Object(&m.Tags)

		return err

	case "metadata":
		err := dec.Object(&m.Metadata)

		return err

	}
	return nil
}

// NKeys returns the number of keys to unmarshal
func (m *NodeMetrics) NKeys() int { return 3 }

// MarshalJSONObject implements MarshalerJSONObject
func (m *NodeMetricsMap) MarshalJSONObject(enc *gojay.Encoder) {
	for k, v := range *m {
		enc.ObjectKey(k, &v)
	}
}

// IsNil checks if instance is nil
func (m *NodeMetricsMap) IsNil() bool {
	return m == nil
}

// UnmarshalJSONObject implements gojay's UnmarshalerJSONObject
func (m *NodeMetricsMap) UnmarshalJSONObject(dec *gojay.Decoder, k string) error {
	var value NodeMetrics
	if err := dec.Object(&value); err != nil {
		return err
	}
	(*m)[k] = value
	return nil
}

// NKeys returns the number of keys to unmarshal
func (m *NodeMetricsMap) NKeys() int { return 0 }

// MarshalJSONObject implements MarshalerJSONObject
func (t *Tags) MarshalJSONObject(enc *gojay.Encoder) {

}

// IsNil checks if instance is nil
func (t *Tags) IsNil() bool {
	return t == nil
}

// UnmarshalJSONObject implements gojay's UnmarshalerJSONObject
func (t *Tags) UnmarshalJSONObject(dec *gojay.Decoder, k string) error {

	switch k {

	}
	return nil
}

// NKeys returns the number of keys to unmarshal
func (t *Tags) NKeys() int { return 0 }

// MarshalJSONObject implements MarshalerJSONObject
func (m *WatcherMetrics) MarshalJSONObject(enc *gojay.Encoder) {
	enc.Int64Key("timestamp", m.Timestamp)
	enc.ObjectKey("window", &m.Window)
	enc.StringKey("source", m.Source)
	enc.ObjectKey("data", &m.Data)
}

// IsNil checks if instance is nil
func (m *WatcherMetrics) IsNil() bool {
	return m == nil
}

// UnmarshalJSONObject implements gojay's UnmarshalerJSONObject
func (m *WatcherMetrics) UnmarshalJSONObject(dec *gojay.Decoder, k string) error {

	switch k {
	case "timestamp":
		return dec.Int64(&m.Timestamp)

	case "window":
		err := dec.Object(&m.Window)

		return err

	case "source":
		return dec.String(&m.Source)

	case "data":
		err := dec.Object(&m.Data)

		return err

	}
	return nil
}

// NKeys returns the number of keys to unmarshal
func (m *WatcherMetrics) NKeys() int { return 4 }

// MarshalJSONObject implements MarshalerJSONObject
func (w *Window) MarshalJSONObject(enc *gojay.Encoder) {
	enc.StringKey("duration", w.Duration)
	enc.Int64Key("start", w.Start)
	enc.Int64Key("end", w.End)
}

// IsNil checks if instance is nil
func (w *Window) IsNil() bool {
	return w == nil
}

// UnmarshalJSONObject implements gojay's UnmarshalerJSONObject
func (w *Window) UnmarshalJSONObject(dec *gojay.Decoder, k string) error {

	switch k {
	case "duration":
		return dec.String(&w.Duration)

	case "start":
		return dec.Int64(&w.Start)

	case "end":
		return dec.Int64(&w.End)

	}
	return nil
}

// NKeys returns the number of keys to unmarshal
func (w *Window) NKeys() int { return 3 }
