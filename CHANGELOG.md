# Changelog

## [0.3.1](https://github.com/kbrockhoff/terraform-aws-lakehouse/compare/v0.3.0...v0.3.1) (2026-11-17)


### Bug Fixes

* **tags:** remove module-created tags except Name and module metadata ([6683792](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/668379215031370f73c24deb417357ef66f70f35))
* **tags:** remove module-created tags except Name and module metadata ([5e11abc](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/5e11abcd313b9135adddc58c2a63b9d67462fe1a))

## [0.3.0](https://github.com/kbrockhoff/terraform-aws-lakehouse/compare/v0.2.0...v0.3.0) (2026-11-16)


### Features

* **alarms:** alarm on failed DDL as well as failed DML ([da0acda](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/da0acda250cdf441a92d63124867a1b9432d17cb))
* **query:** opt-in per-tenant Athena DML workgroup ([02b1dd0](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/02b1dd0b2e2c1dbbb35d3986b15306e23e5aabb1))
* tagging standard, results encryption, DML workgroup, DDL alarms, metadata personas, and verification coverage ([6bba0de](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/6bba0de40aab24262edf748d0cd8e675fffe0780))
* **tenant:** metadata reader and writer personas ([1da3db4](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/1da3db4be1cff19fc201477978a56d068a84f626))
* **verify:** add Lake Formation denial cases for reader_multi DML ([5bf35da](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/5bf35da2d2e57afed68d71f82b1d3034f1c51a94))
* **verify:** detect label-style missing grants and prove Lake Formation denials ([0034fa2](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/0034fa2682f52de0c07921ded6fb54260949c1db))


### Bug Fixes

* **ci:** authenticate and cache tflint plugin downloads ([738aa70](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/738aa70bc3fca3a12e458b86bbe07f6a37e82af2))
* **results:** allow header-less uploads and deny the aws/s3 key on results buckets ([759af02](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/759af02621b55233184d535df4a30cee3507f78b))
* **results:** deny SSE-C uploads on results buckets ([962d1ee](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/962d1ee960845c8e04245077d532f7ddae4c936f))
* **tags:** apply data_tags only to data-at-rest and drop required_tags_config ([ccd3965](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/ccd396527b19297414ad9c1ae911de03e861f5f9))
* **tenant:** restore ComplianceFrameworks on the tenant key ([b31ccb1](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/b31ccb1a88e090d896adc7b8e952ec0e66fcbe27))
* **verify:** recognize Athena's IAM refusal phrasing in denied_by ([9d01a67](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/9d01a67af23ea07780527d59af1da8dd06a64792))
* **verify:** require Lake Formation evidence for denied_by ([cf41ab1](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/cf41ab1f39a006549efa75a6969f535a3ce4b5b7))

## [0.2.0](https://github.com/kbrockhoff/terraform-aws-lakehouse/compare/v0.1.0...v0.2.0) (2026-11-13)


### Features

* support Lake Formation read-only admins ([aab5dc5](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/aab5dc5c10259eff579b74f47a0c087e32c8b454))
* **verify:** add lfverify Lake Formation access verification harness ([70d28e3](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/70d28e3ede3df383b1047bf6ff9a8a415411464e))
* **verify:** gosec scan for lfverify (make lint-verify) ([7daa726](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/7daa726df8274234937df337dd2ed933025e5c11))
* **verify:** standalone lfverify distribution (T060-T070) ([4918ea6](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/4918ea62e989e07cca629c9cc283956100264298))


### Bug Fixes

* address PR [#3](https://github.com/kbrockhoff/terraform-aws-lakehouse/issues/3) review findings ([65969a1](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/65969a10918817108ab89c539f34ec32348b86d1))
* **tenant:** keep row-filtered readers off the unfiltered table grant ([bca2b33](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/bca2b3339b58d5e63d38952b880916f7b530ae9b))
* **verify:** never tear down after a refused preflight (T053) ([a3d5f61](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/a3d5f61a2d8421421da48fad61392c042b2821c6))
* **verify:** probe-mode fixes from the full/probe cross-check (T050, T051) ([2ce3b2a](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/2ce3b2ae02e8fa8472453f5e3491c074772965e7))
* **verify:** record per-case durations; T049 determinism results ([2c59763](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/2c59763aa7a90764a5894ad4b0fd8eef0fd787b2))
* **verify:** sweep all-tables column grants and keep revoking after a failure (T052, T058) ([c90110a](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/c90110a2e63d4f23dfd3732552179fd2185984c1))

## 0.1.0 (2026-10-26)


### Features

* add catalog integration, platform key statements, guards, and LF-tags ([3fa825d](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/3fa825dffe16fe06143f11990f55284b320b7b9b))
* add cost rates, scoped deployer policies, and platform outputs ([1e24784](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/1e2478459bb57808cbd54a1d678f49bc20d14706))
* add iac-tasks-to-beads skill ([7fe0605](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/7fe0605783c9c18e8b5b1ad5f945e300ebcd6f19))
* add platform inputs, environment defaults, and shared data sources ([49317d5](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/49317d54b8521334acd0ab3e2b323f7360198e9d))
* add shared zone buckets, query results bucket, and platform contract ([c49f637](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/c49f637aaa78347d451a55a028c528f9624f462a))
* add the tenant submodule and wire platform-applied tenants ([75eeae9](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/75eeae9872d9ceaa7e5a45055e5bfcef04605b7e))
* zone LF-tag with account-level grants, fixes from the T049 sandbox ([c10bcf3](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/c10bcf34a6b241a60497dcfd540158083b84f60d))


### Bug Fixes

* create data-lake settings before the catalog and drop empty default blocks ([082daa3](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/082daa31293c31cc15d979c5b6c67edbecc030ed))
* deployer and admin permissions, and workgroup deletion, from the T076 sandbox ([7df9c03](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/7df9c03e7e94280898160fd79f82dbdcc3540708))
* deployer shared results access and admin tag grants that revoke cleanly ([949f523](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/949f5236ea2afe43e36dce13e212fce2bb87b3ba))
* fail make lint when TFLint finds errors ([d0a3388](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/d0a3388b72cec29fe7368e6219b5315d45918cd3))
* let CloudWatch alarms use the platform key; decode the SSM contract as nonsensitive ([d404966](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/d4049664f1af405069e423bde936c21f7dd22b3d))
* let stewards create, alter, and drop tables through Glue ([e3a1b25](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/e3a1b257e2f69832640bac0ae509119d1edced85))
* set an empty excluded_column_names on wildcard row filters ([0272f99](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/0272f99b6a18cef4fec02b17fcc7030c0e222beb))
* split the deployer policy, fix KMS and Glue scoping, document admin tag grants ([97e48b4](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/97e48b40e3491869bafc94cc3b7c1d06862ec11d))

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
