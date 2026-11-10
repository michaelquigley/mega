---
title: a failed load never renders as an empty workspace
state: researching
created: 2026-11-09
tags: [ui, honesty]
---

After rebuilding and restarting the service, Michael reloaded the page before it was up and saw an empty project tree, which read as every project and conversation being gone. The store was intact and the service served everything once up; the page had rendered a failed first workspace fetch as "no projects".

Shape: an empty tree is only shown when a successful read returned nothing. Until the first read succeeds the page shows that it is connecting or that the service is unreachable, and once it has had any confirmed state it keeps showing it labelled last known, as the reconnect path already does for runtime marks. The same honesty rule the service applies to its stores, applied to the page's first load.
