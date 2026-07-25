# Issue tracker: GitHub

Issues and specs for this repository live in GitHub Issues at
`TheGeeKing/FileGot`. Use the `gh` CLI for operations.

## Conventions

- Create: `gh issue create --title "..." --body "..."`
- Read: `gh issue view <number> --comments`
- List: `gh issue list --state open`
- Comment: `gh issue comment <number> --body "..."`
- Label: `gh issue edit <number> --add-label "..."`
- Close: `gh issue close <number> --comment "..."`
- Pull requests are not treated as incoming feature requests.

## Skill operations

When a skill says "publish to the issue tracker," create a GitHub issue.
When it says "fetch the relevant ticket," read that issue and its comments.

## Planning

The deferred feature map lives in `docs/roadmap.md`; each capability has its
own GitHub issue. Do not create a combined map issue unless a maintainer asks
for one.

Record every blocking relationship in both places:

1. Add `Blocked by: #<number>` to the blocked issue's description.
2. Add the blocker through GitHub's native `blocked by` relationship.

Do not use description-only dependencies.

Use sub-issues only when a parent issue is deliberately decomposed into work
required to complete that parent. A prerequisite blocker is not the parent of
the work it enables; GitHub rejects that combination as a circular
dependency. The deferred roadmap intentionally has no umbrella issue.

An issue is ready when all blockers are closed and it is unassigned.
