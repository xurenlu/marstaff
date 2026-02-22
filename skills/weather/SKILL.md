---
id: weather
name: Weather
description: Get current weather and forecasts for any location
version: 1.0.0
author: Marstaff
category: utilities
tags: [weather, forecast, temperature]
parameters:
  location:
    type: string
    description: The city name or location
    required: true
  unit:
    type: string
    description: Temperature unit (celsius or fahrenheit)
    default: celsius
---

# Weather Skill

This skill provides weather information for any location worldwide.

## Capabilities

- Current weather conditions
- Temperature (Celsius/Fahrenheit)
- Humidity
- Wind speed and direction
- Weather forecasts

## Usage

### Examples

- "What's the weather in Beijing?"
- "天气怎么样？上海"
- "Get weather forecast for Tokyo"

## Tools

### get_current_weather

Get current weather for a location.

**Parameters:**
- `location` (string, required): City name or location
- `unit` (string, optional): "celsius" or "fahrenheit", defaults to "celsius"

**Returns:** Weather information including temperature, conditions, humidity, and wind.

### get_weather_forecast

Get weather forecast for a location.

**Parameters:**
- `location` (string, required): City name or location
- `days` (integer, optional): Number of days (1-7), defaults to 3
- `unit` (string, optional): "celsius" or "fahrenheit", defaults to "celsius"

**Returns:** Daily forecast with high/low temperatures and conditions.
