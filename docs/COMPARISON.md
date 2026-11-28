# How draft compares

This page compares `draft` with tools people reach for when turning
research into prose. Every cell is either backed by a link or says what
was not measured. It makes no claim of being better at anything that has
no number behind it yet; the public benchmark that would supply those
numbers is on the roadmap (R3.8 in
[`AUDIT-2026-11.md`](AUDIT-2026-11.md)).

Checked on 2026-11-28 against PaperOrchestra commit `d9bae99` and
arXiv:2604.05018. Re-check before quoting: both projects move.

## What each tool is for

| Tool | Input | Output | Job |
| --- | --- | --- | --- |
| `draft` | Someone else's source: PDF, DOCX, TeX, Markdown, text | A Markdown article with a claim ledger and provenance | Write about a source without saying anything it does not |
| [PaperOrchestra][po] | Your own idea and experiment log, plus a LaTeX template | A LaTeX paper with a literature review, figures and a PDF | Turn finished research into a conference submission |
| [STORM][storm] | A topic | A Wikipedia-style article from web search | Survey a topic from the open web |
| [OpenScholar][os] | A question | A cited answer over open-access papers | Answer a research question from the literature |

They overlap less than the names suggest. PaperOrchestra writes *your*
paper; `draft` writes *about* a paper. The comparison below is about the
properties that matter to both: whether the output can be trusted, and
what it takes to run.

## Trust

| Property | `draft` | PaperOrchestra |
| --- | --- | --- |
| A fact reaches the writer only if its quote is verbatim in the source | Yes, deterministic ([ADR 0002](adr/0002-quote-verified-claim-ledger.md)) | No equivalent check. The prompts tell the model the experiment log is "Ground Truth" |
| Every number in a claim must be in its quote | Yes (`claims.Verify`) | Prompt instruction only ("Do not hallucinate numbers") |
| Hedged claims kept hedged | Strength recorded per claim; a hedge restated as fact is flagged (`validate/faithfulness.go`) | Not checked |
| Limitations | Extracted as a claim type and kept in the ledger | The refinement prompt says "Never explicitly state a limitation" (`methods/prompts/content_refinement_agent.py`) |
| What accepts a revision | A revision must pass the same validation gates | A revision is kept when an LLM reviewer's "Overall" score rises (`methods/agents/content_refinement_agent.py`) |
| Citations | Not supported yet: the reference list is dropped when a source is read | Found with Google Search, checked to exist on Semantic Scholar, and written from abstracts cut to 1,500 characters. The writer must cite at least 90% of what was found |
| Sentence-to-source map | Yes, per sentence ([ADR 0008](adr/0008-sentence-attribution-c2pa.md)) | No |
| Signed provenance and an AI-use marking | C2PA manifest, optional signature, human-review record ([ADR 0010](adr/0010-signed-c2pa-credential.md)) | No |

## Running it

| Property | `draft` | PaperOrchestra |
| --- | --- | --- |
| Credentials | None for the default path: it drives a coding-agent CLI you are already signed in to ([ADR 0003](adr/0003-session-cli-backends.md)) | Gemini or OpenAI key, plus a Semantic Scholar key. Literature search calls `gemini-3-flash-preview` with Google Search directly ([issue #7][po7]); a free-tier key gets no Search grounding and an empty bibliography ([issue #63][po63]) |
| Offline | Yes, through Ollama | No |
| Model-written code | Never run; providers get no tools and an empty directory ([ADR 0004](adr/0004-no-tools-granted-to-providers.md)) | Plot code from the model runs through Python `exec()` (`utils/paper_banana_utils.py`) |
| Calls and time per output | Not yet published | About 60 to 70 LLM calls and 39.6 minutes per paper, from the paper |
| Tests and gates | 98% coverage floor, mutation testing on the verifier, fuzzing, complexity gate | No test suite in the repository |

## What draft does not do yet

Said plainly, because a comparison that lists only wins is an advert:

- **Citations and related work.** No bibliography, no reference
  resolution, no literature search.
- **LaTeX or PDF output.** Markdown only.
- **Figures.** None generated or carried over.
- **A published faithfulness score.** The corpus is internal; PaperOrchestra
  at least published a benchmark design, though its data is not released.

[po]: https://github.com/google-research/paper-orchestra
[po7]: https://github.com/google-research/paper-orchestra/issues/7
[po63]: https://github.com/google-research/paper-orchestra/issues/63
[storm]: https://github.com/stanford-oval/storm
[os]: https://github.com/AkariAsai/OpenScholar
