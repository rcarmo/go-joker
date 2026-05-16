#!/usr/bin/env python3
import unittest

import run_benchmarks as rb


class RunBenchmarksParsingTests(unittest.TestCase):
    def test_parse_decimal_ns_per_op(self):
        parsed = rb.parse_output(
            "BenchmarkTiny-6    12345    589.2 ns/op    224 B/op    2 allocs/op\n"
        )
        self.assertIn("BenchmarkTiny", parsed)
        self.assertAlmostEqual(parsed["BenchmarkTiny"].ns_per_op, 589.2)
        self.assertEqual(parsed["BenchmarkTiny"].bytes_per_op, 224)
        self.assertEqual(parsed["BenchmarkTiny"].allocs_per_op, 2)

    def test_parse_integer_ns_per_op(self):
        parsed = rb.parse_output(
            "BenchmarkLoop-6    100    123456 ns/op    16 B/op    1 allocs/op\n"
        )
        self.assertEqual(parsed["BenchmarkLoop"].ns_per_op, 123456)


if __name__ == "__main__":
    unittest.main()
