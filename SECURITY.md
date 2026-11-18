# Security Policy

## Supported Versions

| Version | Supported |
| :------ | :-------: |
| 0.0.x   | Yes       |

## Reporting a Vulnerability

Report security vulnerabilities by opening a
[private advisory](https://github.com/sebastienrousseau/draft/security/advisories/new)
or by emailing **<sebastian.rousseau@gmail.com>**.

Do not open a public issue for security reports.

Include:

- A description of the vulnerability.
- Steps to reproduce.
- Affected versions.
- Any suggested fix (optional).

Expect an initial response within 48 hours. A fix or mitigation plan will follow
within 7 days of confirmation.

## Security Design

`draft` is a local CLI that orchestrates other tools. Its posture:

- **No tokens on disk.** Session backends shell out to an already-authenticated
  agent CLI (`claude`, `copilot`, …) and never read, store or log an API key.
  The opt-in `--engine api:<provider>` escape hatch is the one exception: it
  reads the provider's own key from the environment ([ADR
  0011](docs/adr/0011-direct-api-escape-hatch.md)), and is never chosen
  automatically.
- **No shell interpolation.** External tools (`pdftotext`, `textutil`, the
  provider CLIs, Ollama) are invoked with argument vectors via `os/exec` and
  `net/http`, never through a shell, and always under a cancellable context with
  timeouts.
- **Prompt-injection aware.** Template and source text are quoted to the model as
  untrusted evidence, and the writing prompt explicitly instructs it to ignore
  any instructions found inside them.
- **No configuration from the folder you happen to be in.** A `draft.toml` in
  the working directory, which could pick the engine, the output folder and the
  signing key, is ignored until you trust it with `draft --trust-config`, and an
  edit lapses that trust.
- **Grounding as a control.** Every fact must trace to a verbatim-quote-verified
  claim; ungrounded numbers and silent metric conversions are flagged and
  unverifiable claims are dropped before writing.

## Agent Trust Surface

Online, `draft` drives an AI coding-agent CLI in its non-interactive mode and
asks it only for text. What that agent can do depends on the provider:

- `claude` and `codex` are started with no tools at all, and none of your own
  hooks, plugins or MCP servers.
- An agent driven over ACP (`copilot`, `claude-acp`, …) is refused every
  permission it asks for, and a turn in which it runs a tool anyway is
  cancelled and its answer thrown away.
- `agy` keeps tools no setting removes, but is denied reads outside its
  working directory, commands, file access, URLs and MCP tools for the
  session, and a turn in which it starts a tool is stopped and its answer
  thrown away.
- Any other provider is given no tool-granting flag, but otherwise runs with
  whatever its own configuration allows. Auto mode never picks one; it runs
  only when you name it with `--engine` or pass `--experimental`.

Each runs in an empty temporary directory. Its environment loses `draft`'s own
`DRAFT_*` settings and every credential but its own: variables ending
`_TOKEN`, `_API_KEY`, `_SECRET`, `_PASSWORD` and the like, `AWS_*`, `GH_*` and
`SSH_AUTH_SOCK` are withheld unless the provider signs in with them. That list
is matched by name, so a secret under a name it does not cover still gets
through.
[ADR 0004](docs/adr/0004-no-tools-granted-to-providers.md) records what is
guaranteed and how it was checked. Treat sources as you would any untrusted
input, and prefer the offline Ollama backend (`--engine ollama`) for material
you do not trust.

## Fixed Vulnerabilities

The [November 2026 audit](docs/AUDIT-2026-11.md) found the issues below, apart
from the five fixed in v0.0.48 without an audit ID, which were found in the
work that followed it. Each was held back until its fix was released, and
each fix arrived with a test that failed on the code before it. Upgrade to
v0.0.48 or later to have them all.

| Issue | Severity | Affected | Fixed in |
| --- | --- | --- | --- |
| A claim could smuggle an unverified record into the writer's ledger | Critical | before v0.0.43 | v0.0.43 |
| A run with no verified claims still wrote and saved an article | Critical | before v0.0.43 | v0.0.43 |
| Continuation prompts reached the writer without the verified claims | Critical | before v0.0.43 | v0.0.43 |
| Providers ran with the user's own tools, hooks and MCP servers | High | before v0.0.46 | v0.0.44 (`claude`, `codex`, `copilot`); v0.0.46 (`agy`; `cursor-agent` and `grok` are no longer picked automatically) |
| Every credential in the environment reached every provider | Medium | before v0.0.44 | v0.0.44 |
| A per-call timeout could not stop a hung provider | High | before v0.0.44 | v0.0.44 |
| `--verify` accepted a credential whose data hash excluded the article | High | before v0.0.45 | v0.0.45 |
| A credential vouched for the article but not for the manifest's digests, review or AI disclosure | High | before v0.0.48 | v0.0.48 |
| Deleting the credential made a signed set verify as unsigned | Medium | before v0.0.48 | v0.0.48 |
| A resumed ledger was verified against all sections joined (audit A-M1) | Medium | before v0.0.48 | v0.0.48 |
| A resumed run recorded no ledger digest and skipped the second gate (audit A-M2) | Medium | before v0.0.48 | v0.0.48 |
| A comma, or digits in another script, let a claim's number pass against a quote without it | Medium | before v0.0.48 | v0.0.48 |
| `--review` could add a number no verified claim contains | Medium | before v0.0.48 | v0.0.48 |
| `--verify` reported a certificate as trusted when `c2patool` had not said so | Low | before v0.0.48 | v0.0.48 |

**Ledger record smuggling.** A claim field kept its newlines, and the ledger
the writer reads is line-structured, so text extracted from a crafted source
could print a second, well-formed record whose quote was never checked
against the source. Every field is now put on one line before it is
verified, and a claim that spells out a ledger field is dropped.

**Writing without grounding.** With no claim past the verbatim check, draft
still asked for an article of the minimum length and saved it: ungrounded
prose. A continuation, asked for when an article was cut short, was given
the draft so far but not the verified claims. A run with no verified claims
now fails before the write call, and every continuation carries the claims.

**Provider tools.** draft asked providers only for text, but passing no
tool-granting flag did not take away the tools a CLI loads from its own
configuration. A crafted source could ask the agent to read a file such as
`~/.ssh/id_ed25519` and paraphrase it into a claim, which could reach the
article or the failed-output file. Each provider auto mode can pick now runs
without tools, or is denied them and fails a turn that starts one; see
[Agent Trust Surface](#agent-trust-surface) and
[ADR 0004](docs/adr/0004-no-tools-granted-to-providers.md).

**Credentials.** The provider's environment was filtered only of draft's
own `DRAFT_*` settings. Cloud and VCS tokens, other providers' API keys and
`SSH_AUTH_SOCK` now reach only a provider that signs in with them.

**Hung calls.** A provider whose child process held its output open kept a
call waiting past its timeout, and the processes survived. Every subprocess
now runs in its own process group, which a timeout kills whole.

**C2PA exclusion ranges.** `c2patool` accepts a manifest whose data hash
excludes the whole article and then reports a different article of the same
length as valid, as Buchanan showed in October 2026. `--verify` now fails a
credential that excludes any byte range or does not bind the article by its
data hash, and signing refuses to produce one.

**Unbound manifests.** A credential signs the article's bytes, but the
source and ledger digests, the review record and the AI disclosure live in
the manifest beside it, which nothing tied to the signature. Anyone could
rewrite them, or add a review, and the set still verified as signed and
trusted. `--verify` now requires draft's three assertions in the manifest to
equal the copies the credential signed, and says which one differs.

**Missing credentials.** Deleting the `.c2pa` file turned a signed set into
one that verified as never signed. A signed manifest now records
`signed: true`, so a missing credential fails `--verify`, and
`--require-signature` fails any set without a valid, bound credential.

**Resumed ledgers.** `--resume` checked a saved ledger against every section
joined together, so a quote running from the end of one section, or one
merged paper, into the start of the next passed while occurring in no
source. It also wrote no ledger digest into the manifest and skipped the
second gate. Each record is now verified against one section, as on a fresh
run, and a resumed ledger goes through the same de-duplication, second gate
and digest as a fresh one.

**Number checks.** Every comma was dropped before a claim's numbers were
compared with its quote, so `1,5` grounded `15` and `[1,2,3]` grounded `123`,
and digits in other scripts were not read as numbers at all. Only a
well-formed thousands group now loses its commas, and any script's digits
are compared by value. Separately, `--review` rejected an edit that added an
ungrounded number only under `--strict-numbers`, although "factual
correction" is an edit it allows; it now always rejects one.

**Reported trust.** With no trust list, `c2patool` validates a signature and
says nothing about the certificate, and `--verify` showed that as "valid
and trusted". Trust is now reported only when `c2patool` asserts it.

The ledger smuggling, ungrounded writing, provider tools, credential, resume
and number issues need a source document crafted by an attacker; the hung
call needs a misbehaving provider; and the exclusion range, unbound
manifests and missing credentials need someone able to alter a signed set.
