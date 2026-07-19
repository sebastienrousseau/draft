package prompt

// outputSkeleton is the exact body structure every draft must follow.
const outputSkeleton = `# Title

**Opening thesis paragraph.**

<!-- lead-start -->
<aside class="post-lead" aria-label="Article summary">
<p class="post-lead-tldr"><strong>TL;DR.</strong> ...</p>
<p class="post-lead-heading"><strong>Key takeaways</strong></p>
<ul class="post-lead-takeaways">
  <li><strong>...</strong> ...</li>
</ul>
</aside>
<!-- lead-end -->

> **Executive Summary**
>
> - ...

## First analytical section

...`

// defaultStyleExample is used when no local template directory is available, so
// the writer still receives concrete tone and structure guidance.
const defaultStyleExample = `## Template example: house style

### Heading outline
# A clear, specific, argumentative title
## What the result actually shows
## Why the mechanism matters
## Where it breaks

### Style sample
Strong drafts open with a claim worth defending, not a definition. They stay
concrete: real numbers, named methods, and one vivid example beat a paragraph of
generalities. Paragraphs vary in length, sentences vary in rhythm, and the close
lands a forward-looking point rather than a summary.`
