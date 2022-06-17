# Pg2Ent

generate ent schema from your existing sql file.

* this tool was created to be used in my work only it may contains many bugs, missing many features but I hope it might be useful to you in someway :)

## Installation

```shell
go install github.com/panotza/pg2ent@main
```

## Basic usage

pg2ent read yaml config file name pg2ent.yaml
you can find example config in examples directory\
after configuration is completed. run
```shell
pg2ent
```

## What it can't not do right now

```txt
    - parse sql function arguments
    - ent edges
```