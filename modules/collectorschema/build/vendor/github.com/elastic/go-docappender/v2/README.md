[![ci](https://github.com/elastic/go-docappender/actions/workflows/ci.yml/badge.svg)](https://github.com/elastic/go-docappender/actions/workflows/ci.yml)

# go-docappender

go-docappender provides a Go API for append-only Elasticsearch document indexing.

## License

This software is licensed under the [Apache 2 license](https://github.com/elastic/go-docappender/blob/main/LICENSE).

## Design

`go-docappender` is an evolution of the [Elastic APM Server](https://github.com/elastic/apm-server) Elasticsearch output,
and was formerly known as `modelindexer`.

Prior to 8.0, APM Server used the libbeat Elasticsearch output. 8.0 introduced a new output called "modelindexer", which
was coupled to the APM Server event data model and optimised for APM Server's usage. From 8.0 until 8.5, modelindexer
processed events synchronously and used mutexes for synchronized writes to the cache. This worked well, but didn't seem
to scale well on bigger instances with more CPUs.

```mermaid
flowchart LR;
    subgraph Goroutine
        Flush;
    end
    AgentA & AgentB-->Handler;
    subgraph Intake
    Handler<-->|semaphore|Decode
    Decode-->Batch;
    end
    subgraph ModelIndexer
    Available-.->Active;
    Batch-->Active;
    Active<-->|mutex|Cache;
    end
    Cache-->|FullOrTimer|Flush;

    Flush-->|bulk|ES[(Elasticsearch)];
    Flush-->|done|Available;
```

In APM Server 8.6.0, modelindexer was redesigned to accept events asynchronously, and run one or more "active indexers",
which would each pull events from an in-memory queue and (by default) compress them and write them to a buffer. This
approach reduced lock contention, and allowed for automatically scaling the number of active indexers up and down based
on queue utilisation, with an upper bound based on the available memory.

```mermaid
flowchart LR;
    subgraph Goroutine11
        Flush1(Flush);
    end
    subgraph Goroutine22
        Flush2(Flush);
    end
    AgentA & AgentB-->Handler;
    subgraph Intake
    Handler<-->|semaphore|Decode
    Decode-->Batch;
    end
    subgraph ModelIndexer
    Batch-->Buffer;
    Available;
        subgraph Goroutine1
            Active1(Active);
            Active1(Active)<-->Cache1(Cache);
            Cache1(Cache)-->|FullOrTimer|Flush1(Flush);
        end
        subgraph Goroutine2
            Active2(Active);
            Active2(Active)<-->Cache2(Cache);
            Cache2(Cache)-->|FullOrTimer|Flush2(Flush);
        end
        subgraph Channel
            Buffer-->Active1(Active) & Active2(Active);
        end
        Available-.->Active1(Active) & Active2(Active);
    end

    Flush1(Flush) & Flush2(Flush)-->|bulk|ES[(Elasticsearch)];
    Flush1(Flush) & Flush2(Flush)-->|done|Available;
```

## Releasing

We use GitHub releases to manage tagged releases, and aim to conform to semver
in our release naming.

To create a new release, use the [new release
interface](https://github.com/elastic/go-docappender/releases/new), and use
GitHub's `generate release notes` to get an automatically-generated list of
changes made since the last release.
