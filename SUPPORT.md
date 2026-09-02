# Support

Where to take a question, and what to expect back.

## Choosing a channel

| You want to…                                    | Go to                                                                                                 |
| ----------------------------------------------- | ----------------------------------------------------------------------------------------------------- |
| Understand how something works                  | The [manual](https://draftlib.com), the [README](README.md), or [`docs/`](docs/)                      |
| Check whether your machine is set up            | `draft --doctor` — it reports every requirement and what is missing                                   |
| See what a run would do before committing to it | `draft --dry-run <source>`                                                                            |
| Report a bug                                    | [Bug report](https://github.com/sebastienrousseau/draft/issues/new?template=bug_report.yml)           |
| Request a feature                               | [Feature request](https://github.com/sebastienrousseau/draft/issues/new?template=feature_request.yml) |
| Ask a question or share an idea                 | [Discussions](https://github.com/sebastienrousseau/draft/discussions)                                 |
| Report a security vulnerability                 | **Not an issue** — see [SECURITY.md](SECURITY.md)                                                     |
| Package `draft` for a distribution              | [`docs/packaging.md`](docs/packaging.md), then open an issue                                          |
| Contribute a change                             | [CONTRIBUTING.md](CONTRIBUTING.md) and [DEVELOPMENT.md](DEVELOPMENT.md)                               |

## Before opening an issue

Three commands answer most reports, and including their output turns a
back-and-forth into a fix:

```console
draft --version
draft --doctor
draft --dry-run <your-source>
```

If the problem is a draft that came out wrong, `--json` output and the
`--keep-artifacts` claim ledger say far more than a description of the prose:
the ledger shows exactly which facts the writer was given.

## What to expect

This is a project maintained by one person in their own time. That sets honest
expectations rather than a service level:

- **Security reports** have a stated SLA in [SECURITY.md](SECURITY.md): an
  initial response within 48 hours, a fix or mitigation plan within 7 days of
  confirmation. That commitment is kept ahead of everything else here.
- **Bug reports** are usually acknowledged within a week. A report with a
  reproduction is likely to be fixed; one without may be closed as unactionable
  after a request for detail goes unanswered.
- **Feature requests** are read, and most are declined. `draft` is deliberately
  narrow — see [When not to use draft](README.md#when-not-to-use-draft) — and
  the fastest way to make a case is to show the workflow it unblocks.
- **Questions** in Discussions have no response commitment at all. Other users
  may answer sooner than the maintainer.

Nothing here is a paid support arrangement, and no version carries a support
window: the supported version is the latest release.

## Commercial use

The dual MIT / Apache-2.0 licence permits commercial use without asking. There
is no separate commercial support offering.
