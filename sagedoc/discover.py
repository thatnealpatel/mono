import contextlib
import hashlib
import importlib.metadata
import json
import os
import re
import stat
import sys
import warnings


@contextlib.contextmanager
def _diagnostic_stdout():
    """Route Python and native fd-1 output away from the JSON protocol."""
    sys.stdout.flush()
    sys.stderr.flush()
    saved_stdout = os.dup(1)
    try:
        os.dup2(2, 1)
        with contextlib.redirect_stdout(sys.stderr):
            yield
    finally:
        sys.stderr.flush()
        os.dup2(saved_stdout, 1)
        os.close(saved_stdout)


def _canonical_distribution_name(name):
    return re.sub(r"[-_.]+", "-", name).lower()


def _conda_inventory(prefix):
    """Return every canonical conda package record in deterministic order."""
    records = []
    metadata_dir = os.path.join(os.path.realpath(prefix), "conda-meta")
    try:
        directory_mode = os.lstat(metadata_dir).st_mode
    except FileNotFoundError:
        return records
    if not stat.S_ISDIR(directory_mode):
        raise RuntimeError("conda metadata root is not a directory: " + metadata_dir)
    names = sorted(os.listdir(metadata_dir))
    for filename in names:
        if not filename.endswith(".json"):
            continue
        path = os.path.join(metadata_dir, filename)
        mode = os.lstat(path).st_mode
        if not stat.S_ISREG(mode):
            raise RuntimeError("conda metadata record is not a regular file: " + path)
        with open(path, "rb") as source:
            content = source.read()
        value = json.loads(content.decode("utf-8"))
        if not isinstance(value, dict):
            raise RuntimeError("conda metadata record is not an object: " + path)
        records.append(
            {
                "path": filename,
                "content": hashlib.sha256(content).hexdigest(),
            }
        )
    records.sort(key=lambda item: (item["path"], item["content"]))
    return records


def _distribution_metadata(distribution, filename):
    metadata_root = getattr(distribution, "_path", None)
    if metadata_root is None:
        # Non-filesystem Distribution implementations still participate through
        # the public API; filesystem installations take the stricter path below.
        text = distribution.read_text(filename)
        return None if text is None else hashlib.sha256(text.encode("utf-8")).hexdigest()
    root_path = os.fspath(metadata_root)
    root_mode = os.lstat(root_path).st_mode
    if not stat.S_ISDIR(root_mode):
        # Legacy single-file egg-info metadata has no adjacent RECORD/direct URL.
        if stat.S_ISREG(root_mode):
            text = distribution.read_text(filename)
            return None if text is None else hashlib.sha256(text.encode("utf-8")).hexdigest()
        raise RuntimeError("distribution metadata root is not a regular file or directory: " + root_path)
    path = os.path.join(root_path, filename)
    try:
        mode = os.lstat(path).st_mode
    except FileNotFoundError:
        return None
    if not stat.S_ISREG(mode):
        raise RuntimeError("distribution metadata is not a regular file: " + path)
    with open(path, "rb") as source:
        return hashlib.sha256(source.read()).hexdigest()


def _distribution_inventory():
    records = []
    for distribution in importlib.metadata.distributions():
        name = _canonical_distribution_name(str(distribution.metadata.get("Name") or ""))
        version = str(distribution.version or "")
        location = os.path.realpath(os.fspath(distribution.locate_file("")))
        metadata_root = getattr(distribution, "_path", None)
        metadata_path = "" if metadata_root is None else os.path.realpath(os.fspath(metadata_root))
        records.append(
            {
                "name": name,
                "version": version,
                "location": location,
                "metadata_path": metadata_path,
                # None deliberately distinguishes absence from an empty file.
                "record": _distribution_metadata(distribution, "RECORD"),
                "direct_url": _distribution_metadata(distribution, "direct_url.json"),
            }
        )
    records.sort(
        key=lambda item: (
            item["name"],
            item["version"],
            item["location"],
            item["metadata_path"],
            item["record"] is not None,
            item["record"] or "",
            item["direct_url"] is not None,
            item["direct_url"] or "",
        )
    )
    return records


with _diagnostic_stdout():
    warnings.filterwarnings("ignore")
    import sage

    _roots = list(sage.__path__)
    if len(_roots) != 1:
        raise RuntimeError(
            "unsupported Sage package overlay: expected exactly one sage.__path__ entry, got "
            + str(len(_roots))
        )
    _result = {
        "executable": os.path.realpath(sys.executable),
        "prefix": os.path.realpath(sys.prefix),
        "sage_root": os.path.realpath(os.fspath(_roots[0])),
        "distributions": _distribution_inventory(),
        "conda_records": _conda_inventory(sys.prefix),
    }

json.dump(_result, sys.stdout, ensure_ascii=False, separators=(",", ":"))
sys.stdout.write("\n")
sys.stdout.flush()
