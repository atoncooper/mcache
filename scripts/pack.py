#!/usr/bin/env python3
"""Cross-platform packaging helper — creates tar.gz release archives.

Usage::

    # Binary package
    python scripts/pack.py linux-amd64 --output-dir dist --pkg-name mcache-2.0.0 --bin-dir bin

    # Source package
    python scripts/pack.py --source --output-dir dist --pkg-name mcache-2.0.0

    # Checksums
    python scripts/pack.py --checksums --output-dir dist
"""
import argparse
import glob
import hashlib
import os
import shutil
import sys
import tarfile


def package_one(platform: str, output_dir: str, pkg_name: str, bin_dir: str) -> str:
    """Create a tar.gz for a single platform. Returns the tarball path."""
    ext = ".exe" if "windows" in platform else ""
    bin_src = os.path.join(bin_dir, f"mcache-{platform}{ext}")
    if not os.path.exists(bin_src):
        sys.exit(
            f"ERROR: binary not found: {bin_src}\n"
            f"  Run: make build-{platform}"
        )
    bin_dst = "mcache.exe" if ext else "mcache"
    pkg_dir = os.path.join(output_dir, f"{pkg_name}-{platform}")

    os.makedirs(output_dir, exist_ok=True)
    shutil.rmtree(pkg_dir, ignore_errors=True)
    os.makedirs(pkg_dir)

    shutil.copy2(bin_src, os.path.join(pkg_dir, bin_dst))
    for f in ["config.yaml", "LICENSE", "README.md"]:
        if os.path.exists(f):
            shutil.copy2(f, os.path.join(pkg_dir, f))

    tgz = pkg_dir + ".tar.gz"
    with tarfile.open(tgz, "w:gz") as tf:
        tf.add(pkg_dir, arcname=os.path.basename(pkg_dir))

    shutil.rmtree(pkg_dir, ignore_errors=True)
    print(f"    {tgz}")
    return tgz


# Directories and file patterns to exclude from source tarball.
_SRC_EXCLUDE = {
    "bin", "dist", ".git", "__pycache__", ".pytest_cache",
    ".idea", ".vscode", ".claude", "logs",
    "*.exe", "*.test", "*.out", "*.prof", "*.swp", "*.swo",
    "config.local.yaml", ".DS_Store", "Thumbs.db",
    # Keep the following in sync with .gitignore
    "plans",  # docs/plans/
    "vendor",
    "coverage.out", "profile.out",
}


def _should_include(root: str, name: str) -> bool:
    """Return True if *name* should be included in the source tarball."""
    # Exact directory/file name exclusion
    if name in _SRC_EXCLUDE:
        return False
    # Glob pattern exclusion
    for pat in _SRC_EXCLUDE:
        if "*" in pat or "?" in pat:
            import fnmatch
            if fnmatch.fnmatch(name, pat):
                return False
    # Exclude hidden files/dirs (except .gitignore)
    if name.startswith(".") and name != ".gitignore":
        return False
    return True


def package_source(output_dir: str, pkg_name: str, root_dir: str = ".") -> str:
    """Create a source tarball from the project root."""
    import fnmatch

    pkg_dir = os.path.join(output_dir, pkg_name)
    os.makedirs(output_dir, exist_ok=True)
    shutil.rmtree(pkg_dir, ignore_errors=True)

    # Walk the source tree
    file_count = 0
    for dirpath, dirnames, filenames in os.walk(root_dir):
        # Filter directories in-place
        dirnames[:] = [d for d in dirnames if _should_include(dirpath, d)]
        rel_dir = os.path.relpath(dirpath, root_dir)

        for fname in filenames:
            if not _should_include(dirpath, fname):
                continue
            src = os.path.join(dirpath, fname)
            dst_dir = os.path.join(pkg_dir, rel_dir)
            os.makedirs(dst_dir, exist_ok=True)
            shutil.copy2(src, os.path.join(dst_dir, fname))
            file_count += 1

    tgz = pkg_dir + ".tar.gz"
    with tarfile.open(tgz, "w:gz") as tf:
        tf.add(pkg_dir, arcname=os.path.basename(pkg_dir))

    shutil.rmtree(pkg_dir, ignore_errors=True)
    print(f"    {tgz}  ({file_count} files)")
    return tgz


def generate_checksums(output_dir: str) -> None:
    """Write SHA256SUMS for all tar.gz files in output_dir."""
    pattern = os.path.join(output_dir, "*.tar.gz")
    files = sorted(glob.glob(pattern))
    if not files:
        print("WARNING: no tar.gz files found in", output_dir)
        return

    lines = []
    for f in files:
        h = hashlib.sha256(open(f, "rb").read()).hexdigest()
        lines.append(f"{h}  {os.path.basename(f)}")

    path = os.path.join(output_dir, "SHA256SUMS")
    with open(path, "w") as fp:
        fp.write("\n".join(lines) + "\n")
    print(f"    SHA256SUMS ({len(lines)} files)")


def main() -> None:
    ap = argparse.ArgumentParser(description="mcache release packager")
    ap.add_argument("platform", nargs="?", default="", help="platform label, e.g. linux-amd64")
    ap.add_argument("--source", action="store_true", help="create source tarball instead of binary")
    ap.add_argument("--checksums", action="store_true", help="generate SHA256SUMS instead of packaging")
    ap.add_argument("--output-dir", default="dist")
    ap.add_argument("--pkg-name", default="mcache-dev")
    ap.add_argument("--bin-dir", default="bin")
    args = ap.parse_args()

    if args.checksums:
        generate_checksums(args.output_dir)
    elif args.source:
        package_source(args.output_dir, args.pkg_name)
    elif args.platform:
        package_one(args.platform, args.output_dir, args.pkg_name, args.bin_dir)
    else:
        ap.print_help()
        sys.exit(1)


if __name__ == "__main__":
    main()
