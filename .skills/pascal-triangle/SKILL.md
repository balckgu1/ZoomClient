---
name: pascal-triangle
description: Generate and display Pascal's Triangle using Python. Use when the user asks to generate Pascal's Triangle, compute binomial coefficients, or test the skills module functionality with a Python script.
author: balckgu1
version: v1.0
compatibility: Python 3.6+
---

# Pascal's Triangle Generator

## Overview
This skill generates Pascal's Triangle (杨辉三角) using a Python script. It serves as both a mathematical tool and a test case for verifying the skills module works correctly.

## Quick Start

Run the script to generate Pascal's Triangle:

```bash
python scripts/pascal_triangle.py [rows]
```

- `rows` (optional): Number of rows to generate. Defaults to 10.

## Usage Examples

**Generate default 10 rows:**
```bash
python scripts/pascal_triangle.py
```

**Generate 5 rows:**
```bash
python scripts/pascal_triangle.py 5
```

**Output for 5 rows:**
```
    1
   1 1
  1 2 1
 1 3 3 1
1 4 6 4 1
```

## Workflow

1. Determine the number of rows (default: 10, max: 30)
2. Run: `python scripts/pascal_triangle.py <rows>`
3. The script prints the triangle with aligned formatting

## Mathematical Properties

Each number in Pascal's Triangle is the sum of the two numbers directly above it:
- Row `n`, position `k` = C(n, k) = n! / (k! * (n-k)!)
- The sum of elements in row `n` = 2^n

## Additional Resources

- For more usage examples, see [examples.md](examples.md)
