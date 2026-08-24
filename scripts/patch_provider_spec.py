#!/usr/bin/env python3
# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

"""Post-processes the provider code specification JSON.

Two jobs, both data-driven from gen/provider/spec_overrides.json:

1. DEDUPE: tfplugingen-openapi appends attributes derived from create and
   read operations without merging duplicates (e.g. an attribute present in
   both responses). Duplicate names are merged, keeping the most permissive
   computed_optional_required value:
       required > computed_optional > computed

2. OVERRIDE: apply per-attribute patches keyed by
   "<resource|data_source|provider>.<dotted.attr.path>", e.g. marking
   create-only API tokens as sensitive.

Usage:
    patch_provider_spec.py <provider_code_spec.json> <overrides.json> <output.json>

Run via `task gen:patchspec` between gen:spec and gen:schema.
"""

import json
import sys

COR_RANK = {"required": 3, "computed_optional": 2, "computed": 1}
KINDS = ("string", "int64", "bool", "float64", "number",
         "list", "set", "map", "object",
         "list_nested", "set_nested", "map_nested", "single_nested")


class SpecError(Exception):
    pass


def find_attr(attributes, name):
    for a in attributes:
        if a.get("name") == name:
            return a
    return None


def kind_of(attr):
    for k in KINDS:
        if k in attr:
            return k
    raise SpecError(f"attribute {attr.get('name')!r} has no recognised type key")


def merge_attr(dst, src, path):
    """Merge duplicate attribute definitions; most permissive COR wins."""
    k_dst, k_src = kind_of(dst), kind_of(src)
    if k_dst != k_src:
        raise SpecError(
            f"type mismatch for {path}: {k_dst} vs {k_src}")
    body = dict(dst[k_dst])
    other = src[k_src]

    cor_d = body.get("computed_optional_required")
    cor_s = other.get("computed_optional_required")
    if COR_RANK.get(cor_s, 0) > COR_RANK.get(cor_d, 0):
        body["computed_optional_required"] = cor_s

    # Nested attribute collections are merged recursively by name.
    for nested_key in ("attributes", "nested_object"):
        if nested_key == "nested_object":
            if nested_key in body and nested_key in other:
                inner = body[nested_key].get("attributes", [])
                for child in other[nested_key].get("attributes", []):
                    existing = find_attr(inner, child["name"])
                    if existing is not None:
                        merge_attr(existing, child, f"{path}.{child['name']}")
                    else:
                        inner.append(child)
            elif nested_key in other and nested_key not in body:
                body[nested_key] = other[nested_key]
            continue
        if nested_key in body and nested_key in other:
            for child in other[nested_key]:
                existing = find_attr(body[nested_key], child["name"])
                if existing is not None:
                    merge_attr(existing, child, f"{path}.{child['name']}")
                else:
                    body[nested_key].append(child)
        elif nested_key in other and nested_key not in body:
            body[nested_key] = other[nested_key]

    # Prefer a description that exists; first non-empty wins.
    if not body.get("description") and other.get("description"):
        body["description"] = other["description"]

    dst[k_dst] = body


def dedupe_attributes(attributes, path):
    seen = {}
    out = []
    for attr in attributes:
        name = attr["name"]
        if name in seen:
            merge_attr(seen[name], attr, f"{path}.{name}")
            continue
        seen[name] = attr
        out.append(attr)
        k = kind_of(attr)
        body = attr[k]
        for nested_key in ("attributes",):
            if nested_key in body:
                body[nested_key] = dedupe_attributes(
                    body[nested_key], f"{path}.{name}")
        if "nested_object" in body:
            body["nested_object"]["attributes"] = dedupe_attributes(
                body["nested_object"]["attributes"], f"{path}.{name}[]")
    return out


def resolve_entity(spec, entity_name):
    if entity_name == "provider":
        return spec.get("provider")
    for r in spec.get("resources", []):
        if r["name"] == entity_name:
            return r
    for d in spec.get("datasources", []):
        if d["name"] == entity_name:
            return d
    raise SpecError(f"unknown entity {entity_name!r}")


def apply_override(entity, attr_path, patch):
    parts = attr_path.split(".")
    node = entity["schema"]
    # Walk through nested collections ('foo[]' descends into list elements).
    attributes = node["attributes"]
    target = None
    for i, part in enumerate(parts):
        last = i == len(parts) - 1
        target = find_attr(attributes, part)
        if target is None:
            raise SpecError(f"cannot resolve attribute {attr_path!r}")
        if last:
            break
        k = kind_of(target)
        body = target[k]
        if "nested_object" in body:
            attributes = body["nested_object"]["attributes"]
        elif "attributes" in body:
            attributes = body["attributes"]
        else:
            raise SpecError(f"{attr_path!r}: {part} has no nested attributes")

    k = kind_of(target)
    for key, value in patch.items():
        target[k][key] = value


def main(argv):
    if len(argv) != 4:
        print(__doc__)
        return 2
    spec_path, overrides_path, output_path = argv[1:4]

    with open(spec_path, encoding="utf-8") as fh:
        spec = json.load(fh)
    with open(overrides_path, encoding="utf-8") as fh:
        overrides = json.load(fh)

    for collection in ("resources", "datasources"):
        for entity in spec.get(collection, []):
            attrs = entity["schema"].get("attributes", [])
            entity["schema"]["attributes"] = dedupe_attributes(
                attrs, entity["name"])

    applied = 0
    for entity_name, patches in overrides.get("entities", {}).items():
        entity = resolve_entity(spec, entity_name)
        for attr_path, patch in patches.items():
            apply_override(entity, attr_path, patch)
            applied += 1

    with open(output_path, "w", encoding="utf-8") as fh:
        json.dump(spec, fh, indent=2, ensure_ascii=False)
        fh.write("\n")

    print(f"patched provider spec: overrides applied={applied}")
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main(sys.argv))
    except SpecError as exc:
        print(f"ERROR: {exc}", file=sys.stderr)
        sys.exit(1)
