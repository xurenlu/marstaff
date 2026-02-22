---
id: calculator
name: Calculator
description: Perform mathematical calculations and conversions
version: 1.0.0
author: Marstaff
category: tools
tags: [math, calculator, conversion]
parameters:
  expression:
    type: string
    description: Mathematical expression to evaluate
    required: true
---

# Calculator Skill

This skill provides mathematical calculation capabilities.

## Capabilities

- Basic arithmetic (add, subtract, multiply, divide)
- Advanced operations (power, square root, etc.)
- Unit conversions

## Usage

### Examples

- "Calculate 15% of 280"
- "What is 2^10?"
- "Convert 100 miles to kilometers"
- "sqrt(144) + 5"

## Tools

### calculate

Evaluate a mathematical expression.

**Parameters:**
- `expression` (string, required): Mathematical expression (e.g., "2 + 2", "sqrt(16)", "5 * 10")

**Returns:** Calculation result

### convert

Convert between units.

**Parameters:**
- `value` (number, required): Value to convert
- `from` (string, required): Source unit
- `to` (string, required): Target unit
- `category` (string, required): Category (length, weight, temperature, etc.)

**Returns:** Converted value
