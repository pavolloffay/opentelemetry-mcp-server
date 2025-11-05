/*
 * Copyright 2023 SentinelOne, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package meter_config

import "go.opentelemetry.io/otel/metric"

type MeterConfig struct {
	entity string
	name   string
	meter  *metric.Meter
}

func NewMeterConfig(meter *metric.Meter, entity string, name string) *MeterConfig {
	return &MeterConfig{
		entity: entity,
		name:   name,
		meter:  meter,
	}
}

func (c *MeterConfig) Entity() string {
	return c.entity
}

func (c *MeterConfig) Name() string {
	return c.name
}

func (c *MeterConfig) Meter() *metric.Meter {
	return c.meter
}
