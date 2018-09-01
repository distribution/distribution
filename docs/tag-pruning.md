---
description: High level discussion of tag pruning
keywords: registry, pruning, images, tags, repository, distribution
title: Tag Pruning
---

As of v2.6.0 you can set tag pruning policies on individual repositories that you manage. Based on your specified rules, you can automatically delete unwanted images. In addition to a policy approach, you can also set repository tag limits which limit the number of tags in a specific repository.
 
## About tag pruning

In the context of the Docker registry, tag pruning is the process of deleting image tags but not actual blobs. A garbage collection job takes care of blob deletions.

Additionally repository tag limits are processed in a first in first out manner. For example, if you set a tag limit of 2, adding a third tag would push out the first.

## Tag pruning in practice


### Example



### More details about tag pruning

