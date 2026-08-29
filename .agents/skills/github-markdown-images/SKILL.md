---
name: github-markdown-images
description: README.md files with images must embedd images not in Markdown but in HTML `<img>` tags. Because github.com will not render Markdown images in .md Files.
---

# Use

```markdown
<p align="center">
  <img src="docs/screenshots/basic.png" alt="The Main view: files grouped by detected type with route badges and resolved settings">
</p>
```

# Don't use

```markdown
![The Main view: files grouped by detected type with route badges and resolved settings](docs/screenshots/basic.png)
```
