# CLAUDE.md

For project conventions, build commands, architecture, and contribution
guidelines, see [AGENTS.md](AGENTS.md). Everything below is specific to
the Claude Code operator experience on this repo.

## Identity

This repo uses a project-specific committer identity, not the global
Claude Code identity from `~/.claude/CLAUDE.md`.

- **Name / email**: `thane-developer <thane-developer@macnugget.org>`
- Repo-local git config: `gpg.format=ssh`, `user.signingkey` (path is
  host-local, configure on your machine), `commit.gpgsign=true`

Verify signing is active before your first commit:

```bash
git config commit.gpgsign   # should return true
git config user.email       # should be thane-developer@macnugget.org
git config user.signingkey  # should point at your local public key
```

## GitHub

- GitHub classic token: `/Users/nugget/Sync/Projects/AI/Claude/identity/github_token`
- `gh` should pick this up automatically for this repo.

## CI Gate

**MANDATORY: `just ci` must pass locally before every `git push`. No
exceptions.** Do not rely on GitHub Actions — run the full gate locally
first and fix any issues before pushing. The justfile's `ci` recipe
runs `fmt-check`, `mod-tidy-check`, `vet`, `lint`, `test`, and
`build-all`; it must all pass.

## GitHub Collaboration

Be a good GitHub collaborator. Review threads left open signal
unfinished work — always close the loop. Leave PRs clean and reflective
of reality.

**When addressing review feedback:**
1. Fix the issue in a commit
2. Reply to the thread with the fixing commit hash and a one-line
   explanation
3. Resolve the conversation
4. If deferring (out of scope, follow-up issue), say so explicitly
   before resolving

**After a round of fixes:** Request re-review so the reviewer knows the
ball is back in their court.

**Resolving threads via CLI:**
```bash
gh api graphql -f query='mutation { resolveReviewThread(input: {threadId: "THREAD_ID"}) { thread { isResolved } } }'
```

**PR hygiene:**
- Check off test plan items as they are verified
- Use `Refs #NNN` or `Closes #NNN` in commit bodies
- Keep the PR description accurate as scope evolves
