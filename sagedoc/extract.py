import contextlib
import inspect
import json
import os
import sys
import warnings

_protocol = None


@contextlib.contextmanager
def _diagnostic_stdout():
    """Route Python-level and native fd-1 output to the diagnostic stream."""
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


def _exception_name(exc):
    return type(exc).__name__


def _diagnose(message):
    print("sagedoc extractor: " + message, file=sys.stderr, flush=True)


def _doc_text(value):
    if value is None:
        return ""
    return str(value).strip()


def _qualname(value, kind):
    if kind == "module":
        name = getattr(value, "__name__", None)
        return "" if not name else str(name)
    target = value if kind in ("class", "function") else type(value)
    module = getattr(target, "__module__", None)
    name = getattr(target, "__qualname__", None)
    if not module or not name:
        return ""
    return str(module) + "." + str(name)


def _kind(value):
    if inspect.isclass(value):
        return "class"
    if inspect.ismodule(value):
        return "module"
    if inspect.isroutine(value):
        return "function"
    return "object"


def _resolve_lazy(value, lazy_type):
    seen = set()
    for _ in range(32):
        if not isinstance(value, lazy_type):
            return value
        identity = id(value)
        if identity in seen:
            raise RuntimeError("LazyImport resolution cycle")
        seen.add(identity)
        value = value._get_object()
    raise RuntimeError("LazyImport resolution depth exceeded")


def _file_suffix(value):
    if value is None:
        return ""
    path = os.fsdecode(os.fspath(value)).replace("\\", "/")
    parts = path.split("/")
    for index in range(len(parts) - 1, -1, -1):
        if parts[index] == "sage":
            return "/".join(parts[index:])
    return path


def main():
    global _protocol
    if sys.argv[1:] != ["--jsonl-fd", "3"]:
        _diagnose("usage: extract.py --jsonl-fd 3")
        return 2
    try:
        record_fd = int(sys.argv[2])
        _protocol = os.fdopen(record_fd, "w", encoding="utf-8", buffering=1)
    except (KeyError, OSError, TypeError, ValueError) as exc:
        _diagnose("record protocol: " + _exception_name(exc) + ": " + str(exc))
        return 2

    counts = {
        "public": 0,
        "indexed": 0,
        "none": 0,
        "empty_doc": 0,
        "attribute_failures": 0,
        "lazy_failures": 0,
        "doc_failures": 0,
        "signature_failures": 0,
        "qualname_failures": 0,
        "file_failures": 0,
        "line_failures": 0,
    }

    # Keep fd 1 away from the record protocol even when native Sage code writes
    # directly to it. Records use the inherited descriptor above.
    with _diagnostic_stdout():
        warnings.filterwarnings("ignore")
        try:
            import sage.all as sage_all
            from sage.misc.lazy_import import LazyImport
            from sage.misc.sageinspect import (
                sage_getdef,
                sage_getdoc_original,
                sage_getfile_relative,
                sage_getsourcelines,
            )
            names = sorted(dir(sage_all))
        except Exception as exc:
            _diagnose("fatal import/enumeration: " + _exception_name(exc) + ": " + str(exc))
            return 1

        for name in names:
            if name.startswith("_"):
                continue
            counts["public"] += 1

            try:
                value = getattr(sage_all, name)
            except Exception as exc:
                counts["attribute_failures"] += 1
                _diagnose("omit " + name + ": attribute " + _exception_name(exc))
                continue
            try:
                value = _resolve_lazy(value, LazyImport)
            except Exception as exc:
                counts["lazy_failures"] += 1
                _diagnose("omit " + name + ": lazy resolution " + _exception_name(exc))
                continue
            if value is None:
                counts["none"] += 1
                continue

            original_error = None
            fallback_error = None
            original_returned = False
            fallback_returned = False
            doc = ""
            try:
                doc = _doc_text(sage_getdoc_original(value))
                original_returned = True
            except Exception as exc:
                original_error = exc

            if not doc:
                try:
                    doc = _doc_text(getattr(value, "__doc__"))
                    fallback_returned = True
                except Exception as exc:
                    fallback_error = exc

            if not doc:
                if not original_returned and not fallback_returned:
                    counts["doc_failures"] += 1
                    failures = []
                    if original_error is not None:
                        failures.append(_exception_name(original_error))
                    if fallback_error is not None:
                        failures.append(_exception_name(fallback_error))
                    _diagnose("omit " + name + ": docstring " + ",".join(failures))
                else:
                    counts["empty_doc"] += 1
                    _diagnose("omit " + name + ": empty docstring")
                continue

            kind = _kind(value)

            signature = ""
            try:
                signature = name + str(inspect.signature(value))
            except Exception:
                try:
                    signature = str(sage_getdef(value, name)).strip()
                    if not signature:
                        raise ValueError("empty sage_getdef result")
                except Exception:
                    counts["signature_failures"] += 1
                    signature = ""

            qualname = ""
            try:
                qualname = _qualname(value, kind)
            except Exception:
                counts["qualname_failures"] += 1

            filename = ""
            try:
                filename = _file_suffix(sage_getfile_relative(value))
            except Exception:
                counts["file_failures"] += 1

            line = 0
            try:
                source = sage_getsourcelines(value)
                if source is not None:
                    line = int(source[1])
            except Exception:
                counts["line_failures"] += 1

            record = {
                "name": name,
                "qualname": qualname,
                "kind": kind,
                "signature": signature,
                "docstring": doc,
                "examples": "",
                "file": filename,
                "line": line,
            }
            json.dump(record, _protocol, ensure_ascii=False, separators=(",", ":"))
            _protocol.write("\n")
            _protocol.flush()
            counts["indexed"] += 1

    _diagnose(
        "summary "
        + " ".join(key + "=" + str(value) for key, value in counts.items())
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
