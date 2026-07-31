# Commit messages

Use [Conventional Commits](https://www.conventionalcommits.org/):

```text
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

## Types

| Type | Use for |
|---|---|
| `feat` | A new user-facing capability |
| `fix` | A bug fix |
| `docs` | Documentation only |
| `chore` | Maintenance that is not a fix or feature (ignore rules, tooling noise, issue forms) |
| `build` | Build scripts, packaging, or release-artifact wiring |
| `ci` | CI configuration |
| `refactor` | Code change that is neither fix nor feature |
| `test` | Adding or fixing tests only |
| `perf` | Performance improvement |

## Rules

- Description is imperative, lowercase, no trailing period: `fix: strip empty year parentheses`
- Scope is optional and short: `fix(media): …`, `build(windows): …`
- Body explains **why** when the subject is not enough
- Reference issues in footers when relevant: `Closes #123`
- Do not put multiple unrelated changes in one commit when they can be split cleanly
