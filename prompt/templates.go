// SPDX-FileCopyrightText: 2026 Sebastien Rousseau
// SPDX-License-Identifier: MIT OR Apache-2.0

package prompt

// outputSkeleton is the exact body structure every draft must follow.
const outputSkeleton = `# Title

**...**

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

## ...

...`

// defaultStyleExample is used when no local template directory is available, so
// the writer still receives concrete tone and structure guidance.
const defaultStyleExample = `## Template example: house style

### Headings
Section headings are specific and argumentative: each names its section's point in
the article's own terms, never a generic label. Invent them from the claims; do
not copy any example wording.

### Style sample
Strong drafts open with a claim worth defending, not a definition. They stay
concrete: real numbers, named methods, and one vivid example beat a paragraph of
generalities. Paragraphs vary in length, sentences vary in rhythm, and the close
lands a forward-looking point rather than a summary.`

// houseStyleRules is the single source of the house craft rules. The writing
// prompt embeds it as rules to follow and the review prompt embeds it as
// violations to flag, so the two can never drift. Context-specific guidance —
// the writing prompt's word budget and grounded stance, the review prompt's
// problem checklist — stays in each prompt rather than being duplicated here.
const houseStyleRules = `1. Punctuation and formatting
- Em dashes are allowed when they read naturally.
- No emojis anywhere: headings, subheads, or body.
- Do not reach for lists to organise prose. Use them only when the content is genuinely a set of discrete items. Never bold the opening words of bullets.
- Use contractions.
- Keep headers minimal and plain. Let paragraphs carry the structure.

2. Concreteness
- Use specific, checkable detail: real numbers, names, places, dates, examples.
- Prefer one vivid example over a general claim.
- Cut filler qualifiers: very, really, quite, arguably, essentially, basically.
- If a sentence would survive being deleted, delete it.

3. Rhythm and variance
- Mix sentence lengths naturally; vary paragraph length too.
- Vary how sentences and paragraphs open. Do not start several in a row the same way.
- Do not force choppiness or fragments for drama.
- Do not group things in threes by reflex.

4. Voice
- Take a side. Commit to a claim and defend it.
- Do not hedge every claim.`
