import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]


class BuildScriptsTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="anylink-build-test-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        for name in ("build.sh", "build_docker.sh", "build_web.sh"):
            shutil.copyfile(ROOT / name, self.root / name)
        (self.root / "version").write_text("test-1\n")
        for folder in ("web/ui", "server/ui", "anylink-deploy", "bin"):
            (self.root / folder).mkdir(parents=True)
        (self.root / "web/ui/index.html").write_text("old-ui")
        (self.root / "server/ui/index.html").write_text("old-ui")
        (self.root / "anylink-deploy/old").write_text("keep")
        self.log = self.root / "calls.jsonl"
        self.env = dict(os.environ, PATH=str(self.root / "bin") + os.pathsep + os.environ["PATH"],
                        BUILD_TEST_ROOT=str(self.root), BUILD_TEST_LOG=str(self.log))
        docker = self.root / "bin/docker"
        docker.write_text('''#!/usr/bin/env python3
import json, os, pathlib, sys
args = sys.argv[1:]
with open(os.environ["BUILD_TEST_LOG"], "a") as f:
    f.write(json.dumps(args) + "\\n")
if args[0] == os.environ.get("FAIL_COMMAND"):
    sys.exit(9)
if args[0] == "build":
    pathlib.Path(args[args.index("--iidfile") + 1]).write_text("sha256:" + "a" * 64)
elif args[:2] == ["container", "create"]:
    print("test-container")
elif args[0] == "cp":
    dest = pathlib.Path(args[-1])
    dest.mkdir()
    if args[1].endswith(":/ui"):
        (dest / "index.html").write_text("new-ui")
    else:
        binary = dest / "anylink"
        binary.write_text("#!/bin/sh\\ntouch " + os.environ["BUILD_TEST_ROOT"] + "/HOST_EXECUTED\\n")
        binary.chmod(0o755)
''')
        docker.chmod(0o755)
        git = self.root / "bin/git"
        git.write_text("#!/bin/sh\n[ \"${FAIL_COMMAND:-}\" != git ] || exit 9\nprintf '%s\\n' 0123456789abcdef\n")
        git.chmod(0o755)

    def run_script(self, name, fail=""):
        return subprocess.run(["bash", str(self.root / name)], cwd="/", env=dict(self.env, FAIL_COMMAND=fail),
                              capture_output=True, text=True)

    def calls(self):
        return [json.loads(line) for line in self.log.read_text().splitlines()]

    def test_build_failure_never_exports_or_retags_old_image(self):
        result = self.run_script("build.sh", "build")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual([call[0] for call in self.calls()], ["build"])
        self.assertTrue((self.root / "anylink-deploy/old").exists())
        self.assertFalse((self.root / "anylink-deploy-test-1.tar.gz").exists())

    def test_export_failure_preserves_previous_release(self):
        result = self.run_script("build.sh", "cp")
        self.assertNotEqual(result.returncode, 0)
        self.assertTrue((self.root / "anylink-deploy/old").exists())
        self.assertIn(["container", "rm", "test-container"], self.calls())

    def test_success_exports_exact_image_without_host_execution_or_push(self):
        result = self.run_script("build.sh")
        self.assertEqual(result.returncode, 0, result.stderr)
        create = next(call for call in self.calls() if call[:2] == ["container", "create"])
        self.assertIn("--pull=never", create)
        self.assertIn("--network=none", create)
        self.assertEqual(create[-1], "sha256:" + "a" * 64)
        self.assertFalse(any(call[0] in ("push", "run", "tag") for call in self.calls()))
        self.assertFalse((self.root / "HOST_EXECUTED").exists())
        self.assertTrue((self.root / "anylink-deploy-test-1.tar.gz").exists())

    def test_frontend_failure_preserves_previous_ui(self):
        result = self.run_script("build_web.sh", "build")
        self.assertNotEqual(result.returncode, 0)
        for path in ("web/ui/index.html", "server/ui/index.html"):
            self.assertEqual((self.root / path).read_text(), "old-ui")

    def test_frontend_success_exports_to_both_locations(self):
        result = self.run_script("build_web.sh")
        self.assertEqual(result.returncode, 0, result.stderr)
        for path in ("web/ui/index.html", "server/ui/index.html"):
            self.assertEqual((self.root / path).read_text(), "new-ui")

    def test_missing_ui_stops_before_docker(self):
        (self.root / "web/ui/index.html").unlink()
        result = self.run_script("build.sh")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.log.exists())

    def test_missing_commit_stops_before_docker(self):
        result = self.run_script("build.sh", "git")
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.log.exists())


if __name__ == "__main__":
    unittest.main()
