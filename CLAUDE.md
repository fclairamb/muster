We roughly want to do the same as https://superset.sh & claude-squad: Orchestrate a set of claude code instances.

The issue is:
- Superset is great but eats all my power
- claude-squad is lightweight but has zero cool features

To do that we will:
- Allow to define a set of directories (it can be a repo or a subdir of a repo)
- Allow to create worktrees in them (in a .ssf/worktrees/repo-and-branch-slug)
- Run a claude code instance inside them (one per default, but we could open an other)

It shall be called by running ssf $dir (or ssd .). Either the directory is already in the list or it is added to it.

We shall present the list of directories and their subtrees in the list on the home page with:
- Red if claude is waiting for our input
- Green if the result is ready
- Yellow (or a spinner) if it's working
- White if there's a claude instance on it (without any activity)

It could be something like: (comments are with a //)
- repo1 [main] 🔴 // waiting for an input
- repo2 [main] 🟢 // output ready
- repo3 [main] 🟡 // working
- repo2 [main] ⚪️ // There's a claude instance on it
- dir2 (repo1) // It's the dir2 (which could be third-level dir) of repo1
  - [feat/new-stuff] // a worktree 
- repo1 [main] // No activity

The repo should be ordered by:
- Red
- Green
- Yellow
- White
- The latest opened first

We should be able to search for directories with a /

When we open them with enter, we enter the claude session.
When we are on them we can open the file navigator with "o" and the editor (Zed) with "e".
We should remove it (with confirmation) with "r".
