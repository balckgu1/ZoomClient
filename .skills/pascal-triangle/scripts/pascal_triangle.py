#!/usr/bin/env python3
"""
Pascal's Triangle Generator (杨辉三角生成器)

Generates and displays Pascal's Triangle with aligned formatting.
Each number is the sum of the two numbers directly above it.

Usage:
    python pascal_triangle.py [rows]

Args:
    rows: Number of rows to generate (default: 10, max: 30)
"""

import sys


def generate_pascal_triangle(n: int) -> list[list[int]]:
    """Generate Pascal's Triangle with n rows.

    Args:
        n: Number of rows to generate.

    Returns:
        A list of lists, where each inner list represents a row.
    """
    if n <= 0:
        return []

    triangle = [[1]]
    for i in range(1, n):
        prev_row = triangle[-1]
        new_row = [1]
        for j in range(1, len(prev_row)):
            new_row.append(prev_row[j - 1] + prev_row[j])
        new_row.append(1)
        triangle.append(new_row)

    return triangle


def print_pascal_triangle(triangle: list[list[int]]) -> None:
    """Print Pascal's Triangle with centered alignment.

    Args:
        triangle: The Pascal's Triangle data structure.
    """
    if not triangle:
        print("No rows to display.")
        return

    # Calculate the width of the widest row (last row)
    max_width = len(" ".join(str(x) for x in triangle[-1]))

    for row in triangle:
        row_str = " ".join(str(x) for x in row)
        print(row_str.center(max_width))


def main():
    """Main entry point for the Pascal's Triangle generator."""
    # Parse command line arguments
    rows = 10  # default
    if len(sys.argv) > 1:
        try:
            rows = int(sys.argv[1])
            if rows <= 0:
                print("Error: Number of rows must be a positive integer.")
                sys.exit(1)
            if rows > 30:
                print("Warning: Limiting to 30 rows to avoid excessive output.")
                rows = 30
        except ValueError:
            print(f"Error: Invalid argument '{sys.argv[1]}'. Please provide a positive integer.")
            sys.exit(1)

    print(f"Pascal's Triangle ({rows} rows):")
    print("-" * 40)

    triangle = generate_pascal_triangle(rows)
    print_pascal_triangle(triangle)

    print("-" * 40)
    print(f"Total numbers: {sum(len(row) for row in triangle)}")


if __name__ == "__main__":
    main()
