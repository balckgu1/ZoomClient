# Pascal's Triangle Examples

## Example 1: Default Output (10 rows)

```bash
python scripts/pascal_triangle.py
```

```
Pascal's Triangle (10 rows):
----------------------------------------
                 1
                1 1
               1 2 1
              1 3 3 1
             1 4 6 4 1
           1 5 10 10 5 1
         1 6 15 20 15 6 1
       1 7 21 35 35 21 7 1
     1 8 28 56 70 56 28 8 1
   1 9 36 84 126 126 84 36 9 1
----------------------------------------
Total numbers: 55
```

## Example 2: Small Triangle (5 rows)

```bash
python scripts/pascal_triangle.py 5
```

```
Pascal's Triangle (5 rows):
----------------------------------------
      1
     1 1
    1 2 1
   1 3 3 1
  1 4 6 4 1
----------------------------------------
Total numbers: 15
```

## Example 3: Single Row

```bash
python scripts/pascal_triangle.py 1
```

```
Pascal's Triangle (1 rows):
----------------------------------------
1
----------------------------------------
Total numbers: 1
```

## Example 4: Error Handling

**Invalid argument:**
```bash
python scripts/pascal_triangle.py abc
# Output: Error: Invalid argument 'abc'. Please provide a positive integer.
```

**Negative number:**
```bash
python scripts/pascal_triangle.py -5
# Output: Error: Number of rows must be a positive integer.
```

**Exceeding limit:**
```bash
python scripts/pascal_triangle.py 50
# Output: Warning: Limiting to 30 rows to avoid excessive output.
```

## Mathematical Insights

| Row | Elements | Sum (= 2^n) |
|-----|----------|-------------|
| 0 | 1 | 1 |
| 1 | 1 1 | 2 |
| 2 | 1 2 1 | 4 |
| 3 | 1 3 3 1 | 8 |
| 4 | 1 4 6 4 1 | 16 |
| 5 | 1 5 10 10 5 1 | 32 |

## Use Cases

1. **Binomial Expansion**: (a + b)^n coefficients come from row n
2. **Combinatorics**: C(n, k) = value at row n, position k
3. **Probability**: Used in calculating binomial distributions
