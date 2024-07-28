# Generate MetaData.yaml

This tool generates a `metadata.yml` file from a directory of templates.

If will look for the following comments in the template
- `# cluster-compare-part: <part name>`
- `# cluster-compare-component: <component name>`
- `# cluster-compare-component-required` / `# cluster-compare-component-optional`
- `# cluster-compare-required` / `# cluster-compare-required`

If part or component name are not present it will use the path to for the name.
The component name will be the parent directory and the part name will be the grandparent e.g.
for the path `/home/user/my_reference/abc/123/template.yaml.tmpl`
- the part name will be `abc`
- the component name will be `123`.

In the case of conflicting `required` and `optional` entries within the same component it will default to `required`.
You only need to set - `# cluster-compare-component-required` / `# cluster-compare-component-optional` once per component.

The default if no `require` or `optional` is given is required for both the template and the component.