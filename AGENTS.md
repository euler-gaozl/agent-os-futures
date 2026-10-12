# Service ownership

This repository owns the futures BFF and its Web. Read README.md first.
No database, SQL, migrations, sibling imports or runtime business effects belong here.
Consume only fixed SDK contracts and HTTP APIs; keep vendor and lockfiles reproducible.
Keep service identity server-side; preserve loopback and authenticated Web boundaries.
Source changes need relevant Go/Web tests and independent builds. Do not claim fixtures are live provider evidence.
Never commit credentials, user data or local state. Projects remain under D:/workspace/agent-platform/repos.
