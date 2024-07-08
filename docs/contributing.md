# Contributing

All contributions are valued and welcomed, whether they come in the form of code, documentation, ideas or discussion.
While we have not applied a formal Code of Conduct to this, and related, repositories, we require that all contributors
conduct themselves in a professional and respectful manner.

## Peer review

Although this is an open source project, a review is required from one or more of the people in [OWNERS](../OWNERS)

## Workflow

If you have a problem with the tools or want to suggest a new addition, The first thing to do is create an
Issue for discussion.

When you have a change you want us to include in the main codebase, please open a
Pull Request for your changes and link it to the
associated issue(s).

### Fork and Pull

This project uses the "Fork and Pull" approach for contributions.  In short, this means that collaborators make changes
on their own fork of the repository, then create a Pull Request asking for their changes to be merged into this
repository once they meet our guidelines.

How to create and update your own fork is outside the scope of this document but there are plenty of
[more in-depth](https://gist.github.com/Chaser324/ce0505fbed06b947d962)
[instructions](https://reflectoring.io/github-fork-and-pull/) explaining how to go about this.

Once a change is implemented, tested, documented, and passing all the checks then submit a Pull Request for it to be
reviewed by the OWNERS.  A good Pull Request will be focussed on a single change and broken into
multiple small commits where possible.  As always, you should ensure that tests pass prior to submitting a Pull
Request.  To run the unit tests issue the following command:

```shell
make test
```

Changes are more likely to be accepted if they are made up of small and self-contained commits, which leads on to
the next section.

### Commits

A good commit does a *single* thing, does it completely, and describes *why*.

The commit message should explain both what is being changed, and in the case of anything non-obvious why that change
was made.  Commit messages are again something that has been widely written about, so need not be discussed in detail
here.

Contributors should aim to follow [these seven rules](https://chris.beams.io/posts/git-commit/#seven-rules) and keep individual
commits focussed (`git add -p` will help with this).
