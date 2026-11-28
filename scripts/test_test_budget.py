"""Unit tests for the single-test load correction in test-budget.py.
Run: python3 -I -m unittest discover -s scripts -p test_test_budget.py"""
import importlib.util
import os
import unittest

spec = importlib.util.spec_from_file_location("tb", os.path.join(os.path.dirname(os.path.abspath(__file__)), "test-budget.py"))
tb = importlib.util.module_from_spec(spec)
spec.loader.exec_module(tb)
CAP = 10


def over(elapsed):
    return sorted(k for k, v in tb.leaf_wall(elapsed).items() if v > CAP)


class LeafWall(unittest.TestCase):
    def setUp(self):
        tb.PKG_SPEED.clear()
        tb.FACTOR = 1.0

    def test_idle_waiting_leaf_in_parallel_package_fails(self):
        el = {f"pkg.TestP{i}": 5.0 for i in range(100)}
        el["pkg.TestWaits"] = 12.0
        tb.PKG_SPEED["pkg"] = 1.05  # 20 CPU-s package, near baseline speed
        self.assertEqual(over(el), ["pkg.TestWaits"])

    def test_cpu_leaf_on_a_5x_slower_machine_passes(self):
        tb.PKG_SPEED["pkg"] = 3.0  # clamped slowdown
        self.assertEqual(over({"pkg.TestCPU": 6.5}), [])

    def test_no_calibration_is_raw_wall_time(self):
        self.assertEqual(over({"pkg.TestSlow": 10.5}), ["pkg.TestSlow"])

    def test_faster_machine_never_stretches_a_time(self):
        tb.PKG_SPEED["pkg"] = 0.5
        self.assertEqual(over({"pkg.TestSlow": 10.5}), ["pkg.TestSlow"])


if __name__ == "__main__":
    unittest.main()
