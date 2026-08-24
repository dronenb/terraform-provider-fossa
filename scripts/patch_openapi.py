#!/usr/bin/env python3
# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

"""Deterministic OpenAPI spec patcher for terraform-provider-fossa.

Applies mechanical transforms and rules-driven fixes to the vendored FOSSA
OpenAPI specification so that downstream generators (tfplugingen-openapi,
openapi-generator-cli) can consume it.

Usage:
    patch_openapi.py <upstream.json> <rules.json> <output.json>

The patcher is idempotent and fails loudly (non-zero exit) when:
  - a declared rule path cannot be resolved against the spec
  - a merge encounters conflicting definitions

All fixes are expressed as data in rules.json - never edit patched output by
hand; add a rule instead.
"""

import json
import sys
from copy import deepcopy


class PatchError(Exception):
    pass


# ---------------------------------------------------------------------------
# JSON pointer helpers
# ---------------------------------------------------------------------------

def parse_pointer(pointer):
    """Parse an RFC 6901-style pointer into tokens ('[n]' selects array index)."""
    if pointer == "":
        return []
    if not pointer.startswith("/"):
        raise PatchError(f"pointer must start with '/': {pointer}")
    tokens = []
    for raw in pointer.lstrip("/").split("/"):
        tok = raw.replace("~1", "/").replace("~0", "~")
        if tok.startswith("[") and tok.endswith("]"):
            tokens.append(int(tok[1:-1]))
        else:
            tokens.append(tok)
    return tokens


def resolve(node, pointer):
    cur = node
    for tok in parse_pointer(pointer):
        if isinstance(tok, int):
            if not isinstance(cur, list):
                raise PatchError(f"expected array at token {tok} in {pointer}")
            try:
                cur = cur[tok]
            except IndexError:
                raise PatchError(f"index {tok} out of range in {pointer}")
        else:
            if not isinstance(cur, dict) or tok not in cur:
                raise PatchError(f"cannot resolve '{tok}' in {pointer}")
            cur = cur[tok]
    return cur


def resolve_parent(node, pointer):
    tokens = parse_pointer(pointer)
    parent_ptr = "".join(
        f"[{t}]" if isinstance(t, int) else f"/{str(t).replace('~', '~0').replace('/', '~1')}"
        for t in tokens[:-1]
    )
    return resolve(node, parent_ptr), tokens[-1]


# ---------------------------------------------------------------------------
# Schema merging primitives
# ---------------------------------------------------------------------------

# Keys whose conflicting values are benign (documentation/metadata); first
# definition wins instead of failing the merge.
TOLERATED_KEYS = {"description", "example", "title", "summary", "deprecated"}


def _is_tolerated(key):
    return key in TOLERATED_KEYS or key.startswith("x-")


def merge_schemas(a, b, *, required_mode="union", path="<root>"):
    """Recursively merge OpenAPI schema dict b into a and return the result."""
    out = deepcopy(a)
    for key, bval in b.items():
        if key not in out:
            out[key] = deepcopy(bval)
            continue
        aval = out[key]

        if key == "properties":
            merged = deepcopy(aval)
            for pname, pschema in bval.items():
                if pname in merged:
                    merged[pname] = merge_schemas(
                        merged[pname], pschema, required_mode=required_mode,
                        path=f"{path}.{pname}",
                    )
                else:
                    merged[pname] = deepcopy(pschema)
            out[key] = merged
        elif key == "required":
            if required_mode == "intersection":
                out[key] = sorted(set(aval) & set(bval))
            else:
                out[key] = sorted(set(aval) | set(bval))
        elif key == "enum":
            merged = list(aval)
            for v in bval:
                if v not in merged:
                    merged.append(v)
            out[key] = merged
        elif key == "items":
            out[key] = merge_schemas(aval, bval, required_mode=required_mode, path=f"{path}[]")
        elif key in ("oneOf", "anyOf", "allOf"):
            # Nested composition keywords inside branches being merged are
            # appended; callers should have resolved these first wherever they
            # matter.
            out[key] = aval + [s for s in bval if s not in aval]
        elif key == "type":
            # A bare {"type":"null"} marks an absent/optional value in the
            # upstream spec; reconcile with the concrete side as nullable.
            a_null = aval == "null"
            b_null = bval == "null"
            if a_null and b_null:
                pass
            elif a_null or b_null:
                out[key] = bval if a_null else aval
                out["nullable"] = True
            elif aval != bval:
                raise PatchError(f"conflict at {path}: {key!r}: {aval!r} != {bval!r}")
        elif _is_tolerated(key):
            pass  # first definition wins
        elif aval != bval:
            raise PatchError(
                f"conflict at {path}: {key!r}: {aval!r} != {bval!r}"
            )
    return out


def merge_composition(schema_node, keyword, *, required_mode="union", path="<root>"):
    """Collapse schema_node[keyword] (list of schemas) into a single schema."""
    parts = schema_node.pop(keyword)
    if not parts:
        raise PatchError(f"empty {keyword} at {path}")
    merged = deepcopy(parts[0])
    for part in parts[1:]:
        merged = merge_schemas(merged, part, required_mode=required_mode, path=path)
    rest = {k: v for k, v in schema_node.items() if k != keyword}
    result = merge_schemas(merged, rest, required_mode=required_mode, path=path)
    return result


# ---------------------------------------------------------------------------
# Global transforms
# ---------------------------------------------------------------------------

def transform_flatten_unions(node, stats):
    """Rewrite type arrays ['string','null'] -> type:'string', nullable:true."""
    if isinstance(node, dict):
        t = node.get("type")
        if isinstance(t, list):
            types_ = [x for x in t if x != "null"]
            nullable = len(types_) != len(t)
            if len(types_) != 1:
                raise PatchError(f"cannot flatten multi-type {t!r} (only T|null supported)")
            node["type"] = types_[0]
            if nullable:
                node["nullable"] = True
                stats["unions"] += 1
        for value in node.values():
            transform_flatten_unions(value, stats)
    elif isinstance(node, list):
        for item in node:
            transform_flatten_unions(item, stats)


def transform_merge_allof(node, stats, path="$"):
    """Recursively collapse every allOf found beneath node."""
    if isinstance(node, dict):
        # Merging can surface further allOf keywords from nested subschemas,
        # so collapse repeatedly until this node is composition-free.
        while isinstance(node.get("allOf"), list):
            merged = merge_composition(node, "allOf", required_mode="union", path=path)
            node.clear()
            node.update(merged)
            stats["allofs"] += 1
        for key, value in list(node.items()):
            transform_merge_allof(value, stats, f"{path}/{key}")
    elif isinstance(node, list):
        for i, item in enumerate(node):
            transform_merge_allof(item, stats, f"{path}[{i}]")


def transform_collapse_primitive_oneof(node, stats):
    """Collapse oneOf branches that are all bare primitive schemas into one.

    Example: oneOf[{type:string},{type:number},{type:boolean}] -> {type:string}
    The first branch's type wins; sibling keywords are preserved.
    """
    if isinstance(node, dict):
        branches = node.get("oneOf")
        if isinstance(branches, list) and branches:
            primitives = []
            for branch in branches:
                if (
                    isinstance(branch, dict)
                    and set(branch) <= {"type", "nullable", "format"}
                    and isinstance(branch.get("type"), str)
                    and branch["type"] in ("string", "number", "integer", "boolean")
                ):
                    primitives.append(branch)
                else:
                    break
            if len(primitives) == len(branches) == 1:
                node.pop("oneOf")
                node.update(primitives[0])
                stats["primitive_oneOf"] += 1
            elif len(primitives) == len(branches):
                node.pop("oneOf")
                node.update({"type": primitives[0]["type"]})
                stats["primitive_oneOf"] += 1
        # Always descend into every child (including any surviving oneOf
        # branches) so nested occurrences are also collapsed.
        for value in list(node.values()):
            transform_collapse_primitive_oneof(value, stats)
    elif isinstance(node, list):
        for item in node:
            transform_collapse_primitive_oneof(item, stats)


def _is_schema_node(node):
    """Heuristic: OAS Schema Objects vs Parameter/RequestBody objects."""
    return (
        isinstance(node, dict)
        and "in" not in node
        and any(
            k in node
            for k in ("properties", "items", "$ref", "additionalProperties")
        )
    ) or (
        isinstance(node, dict)
        and "type" in node
        and "name" not in node
    )


def transform_normalize_required(node, stats):
    """Remove invalid non-array `required` from Schema Objects.

    The upstream spec emits boolean `required` on some schema objects
    (legal on parameters, illegal on schemas); openapi-generator rejects it.
    """
    if isinstance(node, dict):
        req = node.get("required")
        if "required" in node and not isinstance(req, list) and _is_schema_node(node):
            del node["required"]
            stats["schema_required"] += 1
        for value in node.values():
            transform_normalize_required(value, stats)
    elif isinstance(node, list):
        for item in node:
            transform_normalize_required(item, stats)


def transform_normalize_contact_url(spec, stats):
    contact = spec.get("info", {}).get("contact", {})
    url = contact.get("url")
    if url and not str(url).startswith(("http://", "https://")):
        contact["url"] = f"https://{url}"
        stats["contact_url"] += 1


def transform_dedupe_operation_ids(spec, stats):
    """Rename duplicated operationIds deterministically (first wins).

    The upstream spec reuses some operationIds across paths; openapi-generator
    emits one Go file per id and fails to compile on duplicates.
    """
    seen = {}
    for path, item in spec.get("paths", {}).items():
        if not isinstance(item, dict):
            continue
        for method, op in item.items():
            if method not in ("get", "put", "post", "delete", "patch", "options", "head", "trace"):
                continue
            op_id = op.get("operationId")
            if not op_id:
                continue
            if op_id in seen:
                n = seen[op_id] + 1
                seen[op_id] = n
                new_id = f"{op_id}_{n}"
                op["operationId"] = new_id
                stats["dup_operation_ids"] += 1
                print(
                    f"renamed duplicate operationId {op_id!r} at {method.upper()} {path} -> {new_id}",
                    file=sys.stderr,
                )
            else:
                seen[op_id] = 1


def transform_single_tag_operations(spec, stats):
    """Keep only the first tag on multi-tag operations.

    openapi-generator emits one service method per tag; an operation tagged
    twice is generated twice and fails to compile.
    """
    for path, item in spec.get("paths", {}).items():
        if not isinstance(item, dict):
            continue
        for method, op in item.items():
            if method not in ("get", "put", "post", "delete", "patch", "options", "head", "trace"):
                continue
            tags = op.get("tags")
            if isinstance(tags, list) and len(tags) > 1:
                op["tags"] = tags[:1]
                stats["multi_tag_ops"] += 1


def transform_normalize_json_type(node, stats):
    """Replace the upstream-invented {"type":"json"} with an any-schema.

    'json' is not a valid OAS type; openapi-generator emits an undefined
    Json symbol for it. An empty subschema yields map[string]interface{}.
    """
    if isinstance(node, dict):
        if node.get("type") == "json" and set(node) <= {"type", "description"}:
            desc = node.get("description")
            node.clear()
            if desc:
                node["description"] = desc
            stats["json_types"] += 1
        for value in list(node.values()):
            transform_normalize_json_type(value, stats)
    elif isinstance(node, list):
        for item in node:
            transform_normalize_json_type(item, stats)


def transform_drop_shadowed_ids(node, stats):
    """Remove '_id' properties where a sibling 'id' exists.

    Both sanitise to the same Go identifier and fail to compile.
    """
    if isinstance(node, dict):
        props = node.get("properties")
        if isinstance(props, dict) and "id" in props and "_id" in props:
            del props["_id"]
            req = node.get("required")
            if isinstance(req, list) and "_id" in req:
                req.remove("_id")
            stats["shadowed_ids"] += 1
        for value in list(node.values()):
            transform_drop_shadowed_ids(value, stats)
    elif isinstance(node, list):
        for item in node:
            transform_drop_shadowed_ids(item, stats)


def transform_rename_boolean_has_props(node, stats):
    """Rename 'has<X>' boolean properties that collide with siblings.

    openapi-generator derives a Has<GoName>() nil-check method from every
    field, so a sibling pair like url + hasUrl compiles to both field HasUrl
    and method HasUrl(). The go client ignores x-GoName, so the JSON property
    itself must be renamed; only genuinely-colliding properties are touched
    to keep wire compatibility everywhere else.
    """
    if isinstance(node, dict):
        props = node.get("properties")
        if isinstance(props, dict):
            siblings = {k.lower() for k in props}
            for pname in list(props):
                pschema = props[pname]
                if (
                    isinstance(pschema, dict)
                    and pschema.get("type") == "boolean"
                    and pname.startswith("has")
                    and len(pname) > 3
                    and pname[3].isupper()
                    and pname[3:].lower() in siblings
                ):
                    new_name = f"{pname[3:]}Present"
                    props[new_name] = props.pop(pname)
                    stats["has_props"] += 1
                    print(
                        f"renamed colliding property {pname!r} -> {new_name!r}",
                        file=sys.stderr,
                    )
        req = node.get("required")
        if isinstance(req, list) and any(
            isinstance(r, str) and r.startswith("has") and r[3:].lower() in {k.lower() for k in props}
            for r in req
        ) if props else False:
            node["required"] = [
                f"{r[3:]}Present"
                if isinstance(r, str) and r.startswith("has") and len(r) > 3
                and r[3].isupper() and r[3:].lower() in {k.lower() for k in props}
                else r
                for r in req
            ]
        for value in list(node.values()):
            transform_rename_boolean_has_props(value, stats)
    elif isinstance(node, list):
        for item in node:
            transform_rename_boolean_has_props(item, stats)


# ---------------------------------------------------------------------------
# Rules-driven oneOf resolution
# ---------------------------------------------------------------------------

def pick_branch(branches, rule, pointer):
    match = rule.get("match")
    if not match:
        raise PatchError(f"pick_branch at {pointer} requires 'match'")
    for i, branch in enumerate(branches):
        ok = True
        for sub_path, expected in match.items():
            try:
                actual = resolve(branch, sub_path if sub_path.startswith("/") else f"/{sub_path}")
            except PatchError:
                ok = False
                break
            if actual != expected:
                ok = False
                break
        if ok:
            return i, deepcopy(branch)
    raise PatchError(f"pick_branch at {pointer}: no branch matches {match}")


def apply_oneof_rule(spec, rule, stats):
    pointer = rule["path"]
    strategy = rule["strategy"]
    target = resolve(spec, pointer)

    if not isinstance(target, dict) or "oneOf" not in target:
        raise PatchError(f"{pointer}: no oneOf present (already resolved?)")

    branches = target.pop("oneOf")

    if strategy == "merge_all_branches":
        merged = deepcopy(branches[0])
        for branch in branches[1:]:
            merged = merge_schemas(
                merged, branch, required_mode="intersection",
                path=pointer,
            )
        result = merged
    elif strategy == "pick_branch":
        _, result = pick_branch(branches, rule, pointer)
    elif strategy == "replace_schema":
        if "schema" not in rule:
            raise PatchError(f"{pointer}: replace_schema requires 'schema'")
        result = deepcopy(rule["schema"])
    else:
        raise PatchError(f"{pointer}: unknown strategy {strategy!r}")

    # Preserve any sibling keywords that were not part of oneOf handling.
    siblings = {k: v for k, v in target.items()}
    result_full = {**result, **siblings}
    parent, key = resolve_parent(spec, pointer)
    parent[key] = result_full
    stats["oneOf"] += 1


def apply_remove_rule(spec, pointer, removed):
    parent, key = resolve_parent(spec, pointer)
    if not isinstance(parent, dict) or key not in parent:
        raise PatchError(f"remove: cannot resolve {pointer}")
    del parent[key]
    removed.append(pointer)


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main(argv):
    if len(argv) != 4:
        print(__doc__)
        return 2

    upstream_path, rules_path, output_path = argv[1], argv[2], argv[3]

    with open(upstream_path, encoding="utf-8") as fh:
        spec = json.load(fh)
    with open(rules_path, encoding="utf-8") as fh:
        rules = json.load(fh)

    stats = {
        "unions": 0,
        "allofs": 0,
        "oneOf": 0,
        "primitive_oneOf": 0,
        "schema_required": 0,
        "contact_url": 0,
        "dup_operation_ids": 0,
        "multi_tag_ops": 0,
        "json_types": 0,
        "shadowed_ids": 0,
        "has_props": 0,
    }
    removed = []

    if rules.get("rename_boolean_has_props"):
        transform_rename_boolean_has_props(spec, stats)

    if rules.get("normalize_json_type"):
        transform_normalize_json_type(spec, stats)

    if rules.get("drop_shadowed_ids"):
        transform_drop_shadowed_ids(spec, stats)

    if rules.get("dedupe_operation_ids"):
        transform_dedupe_operation_ids(spec, stats)

    if rules.get("single_tag_operations"):
        transform_single_tag_operations(spec, stats)

    if rules.get("flatten_unions"):
        transform_flatten_unions(spec, stats)

    if rules.get("merge_allof"):
        transform_merge_allof(spec, stats)

    if rules.get("collapse_primitive_oneOf"):
        transform_collapse_primitive_oneof(spec, stats)

    if rules.get("normalize_schema_required"):
        transform_normalize_required(spec, stats)

    transform_normalize_contact_url(spec, stats)

    for rule in rules.get("resolve_oneOf", []):
        apply_oneof_rule(spec, rule, stats)

    for pointer in rules.get("remove", []):
        apply_remove_rule(spec, pointer, removed)

    with open(output_path, "w", encoding="utf-8") as fh:
        json.dump(spec, fh, indent=1, ensure_ascii=False)
        fh.write("\n")

    print(
        f"patched: flattened={stats['unions']} merged_allof={stats['allofs']} "
        f"primitive_oneOf={stats['primitive_oneOf']} "
        f"schema_required_fixed={stats['schema_required']} "
        f"contact_url_fixed={stats['contact_url']} "
        f"dup_operation_ids={stats['dup_operation_ids']} "
        f"multi_tag_ops={stats['multi_tag_ops']} "
        f"json_types={stats['json_types']} shadowed_ids={stats['shadowed_ids']} "
        f"has_props={stats['has_props']} "
        f"resolved_oneOf={stats['oneOf']} removed={len(removed)}"
    )
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main(sys.argv))
    except PatchError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        sys.exit(1)
