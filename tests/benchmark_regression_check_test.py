import importlib.util
import tempfile
import subprocess
import sys
import unittest
from pathlib import Path

SPEC = importlib.util.spec_from_file_location(
    "benchmark_regression_check", Path(__file__).with_name("benchmark_regression_check.py")
)
MODULE = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(MODULE)


class BenchmarkRegressionCheckTest(unittest.TestCase):
    def test_parse_groups_cpu_suffix_samples(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "bench.txt"
            path.write_text(
                "BenchmarkHot-6 100 12.5 ns/op 16 B/op 1 allocs/op\n"
                "BenchmarkHot-6 100 13.5 ns/op 16 B/op 1 allocs/op\n"
            )
            parsed = MODULE.parse(path)
        self.assertEqual(parsed["BenchmarkHot"]["ns"], [12.5, 13.5])
        self.assertEqual(parsed["BenchmarkHot"]["bytes"], [16.0, 16.0])

    def test_parse_throughput_subbenchmark(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "bench.txt"
            path.write_text("BenchmarkASCII/1024-6 1000 266.0 ns/op 3849.62 MB/s 110 B/op 2 allocs/op\n")
            parsed = MODULE.parse(path)
        self.assertEqual(parsed["BenchmarkASCII/1024"]["ns"], [266.0])
        self.assertEqual(parsed["BenchmarkASCII/1024"]["allocs"], [2.0])

    def test_empty_samples_fail_closed(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "empty.txt"
            path.write_text("PASS\n")
            result = subprocess.run([sys.executable, str(Path(__file__).with_name("benchmark_regression_check.py")), str(path), str(path)], capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("no benchmark samples", result.stderr)

    def test_cv_is_zero_for_constant_samples(self):
        self.assertEqual(MODULE.cv([10.0] * 6), 0.0)


if __name__ == "__main__":
    unittest.main()
