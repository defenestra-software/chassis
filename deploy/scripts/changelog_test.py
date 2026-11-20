import os
from pathlib import Path
import subprocess
import tempfile
import unittest
import xml.etree.ElementTree as ET


SCRIPT = Path(__file__).with_name("changelog.sh")


class ChangelogDatesTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.changelog = Path(self.temp.name) / "CHANGELOG.md"

    def run_command(self, *args):
        return subprocess.run(
            ["bash", str(SCRIPT), *args],
            env={**os.environ, "CHANGELOG": str(self.changelog)},
            capture_output=True,
            text=True,
        )

    def write_release(self, date="", prefix=""):
        suffix = f" - {date}" if date else ""
        self.changelog.write_text(
            f"# Changelog\n\n{prefix}## [0.1.0]{suffix}\n\n### Added\n\n- First release.\n"
        )

    def assert_exports(self, date):
        checked = self.run_command("check", "0.1.0")
        self.assertEqual(checked.returncode, 0, checked.stderr)
        rpm = self.run_command("rpm", "0.1.0", "1")
        self.assertEqual(rpm.returncode, 0, rpm.stderr)
        self.assertIn(" - 0.1.0-1", rpm.stdout)
        self.assertRegex(rpm.stdout, r"^\* \w{3} \w{3} \d{2} \d{4} ")
        appstream = self.run_command("appstream")
        self.assertEqual(appstream.returncode, 0, appstream.stderr)
        release = ET.fromstring(appstream.stdout).find("release")
        self.assertEqual(release.get("date"), date)

    def test_roll_adds_date_to_existing_release(self):
        for prefix in ("", "## [Unreleased]\n\n"):
            with self.subTest(prefix=prefix):
                self.write_release(prefix=prefix)
                result = self.run_command("roll", "0.1.0", "2026-11-04")
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assert_exports("2026-11-04")

    def test_roll_preserves_existing_date(self):
        self.write_release("2026-10-31")
        result = self.run_command("roll", "0.1.0", "2026-11-04")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assert_exports("2026-10-31")

    def test_roll_unreleased(self):
        self.changelog.write_text("# Changelog\n\n## [Unreleased]\n\n- First release.\n")
        result = self.run_command("roll", "0.1.0", "2026-11-04")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assert_exports("2026-11-04")

    def test_checks_and_exports_reject_missing_or_invalid_dates(self):
        for date in ("", "2026-02-29", "2026-04-31", "2026-00-12", "2026-13-01"):
            for args in (("check", "0.1.0"), ("rpm", "0.1.0", "1"), ("appstream",)):
                with self.subTest(date=date, args=args):
                    self.write_release(date)
                    result = self.run_command(*args)
                    self.assertNotEqual(result.returncode, 0)
                    self.assertEqual(result.stdout, "")
                    self.assertIn("valid YYYY-MM-DD date", result.stderr)

    def test_invalid_roll_date_leaves_file_unchanged(self):
        self.write_release()
        before = self.changelog.read_bytes()
        result = self.run_command("roll", "0.1.0", "2026-02-29")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.changelog.read_bytes(), before)

    def test_leap_day(self):
        self.write_release("2024-02-29")
        self.assert_exports("2024-02-29")


if __name__ == "__main__":
    unittest.main()
