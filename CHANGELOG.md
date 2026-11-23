# Changelog

## [0.5.0](https://github.com/kbrockhoff/terraform-aws-lakehouse/compare/v0.4.0...v0.5.0) (2026-11-23)


### Features

* **settings:** optional Lake Formation Identity Center integration (FR-032) ([b664cc5](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/b664cc580bf5c209193d1f818ec72f9293d13909))


### Bug Fixes

* **kms:** explicit key administrators, scoped delegations, and review fixes ([71ce761](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/71ce761db8047993f32268d2528c93add51704af))
* **kms:** limit Athena results key use to the platform's S3 buckets ([212a46e](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/212a46e91a90e017f0187da02735bfb5c8ae4b0b))
* **kms:** name key administrators and limit account root to read-only ([7e43eb3](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/7e43eb32df07be9549ab143121acd1e55b807702))
* **tenant:** validate key administrator account without indexing the ARN ([8ce33f6](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/8ce33f65b6a0e37828993accc9371e4f0d93e29e))
* **test:** install test providers from a local mirror ([2e8e67b](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/2e8e67bda0b0b0999a558fc4ebf838bd30974c78))
* **test:** skip Pricing API lookups outside cost-focused tests ([c7f557f](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/c7f557fae844839195c365700f8d97c2e7ca8c20))
* **test:** stop Pricing API and registry flakes in make test ([34b3991](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/34b3991c316f9866df21676de510d60029b0961f))
* **variables:** remove deprecated catalog_integration_config.allow_full_table_external_data_access ([cca0d0c](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/cca0d0caaeacbc2e09e814a4d29b12d0247e76f1))

## [0.4.0](https://github.com/kbrockhoff/terraform-aws-lakehouse/compare/v0.3.1...v0.4.0) (2026-11-19)


### Features

* **deployer:** never allow deleting s3tablescatalog; add catalog tests (T078, T079) ([a6f5c97](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/a6f5c9776d3d06bfa5e2822ef1894d1214f0c740))
* harden catalog and Lake Formation account settings (spec 003) ([5326130](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/53261304683e934c663d5f0eddf848a4aa3a15fb))
* **ops:** extend deployer, cost estimate, and outputs for account settings ([6d02765](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/6d0276572cab344bea58e03ee618a6d840d8288e))
* **settings:** add 003 inputs and record sandbox confirmation results ([a801a06](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/a801a06725bf1d98b4bc4b778e451dd70a40e284))
* **settings:** add account-settings submodule behind an owner flag ([6525d15](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/6525d153810e0617fbf929d36591ca0c8a4f5233))
* **settings:** add s3tablescatalog check-and-create (T074, T075) ([6bfc989](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/6bfc989144727bc4090537cf326b8a7dd698ac3b))
* **settings:** create s3tablescatalog before any other resource ([081dea6](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/081dea6883099718cceb60e95ac7ae60b48cacaa))
* **settings:** move s3tablescatalog out of catalog-integration (T076, T077) ([31e4ab1](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/31e4ab1a5aab20984138132e230200ae0a6e37a0))
* **storage:** deny direct table access and extend the platform contract ([9e625ed](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/9e625ed74c65ec6351471829f554433857b338a9))
* **tenant:** add cross-account shares and dedicated bucket deny ([dd19145](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/dd19145379c690c70fae5be8b0885c6a96675aaa))
* **verify:** add bypass persona and account settings checks to lfverify ([1154fad](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/1154fadebbec22e0a9c5e0a9f86d6e1d14368574))
* **verify:** add consumer-account sharing cases to lfverify ([aec7cc2](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/aec7cc21043128b4d7c996850623e0911a8d25de))
* **verify:** add consumer-admin command and fix resource link drift ([f13d37d](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/f13d37dc20a89b065f0956f6c0eb7c41498733f3))
* **verify:** add upgrade test and bypass_exempt overlay, close Phase 7 ([6440f84](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/6440f8470dec56c0d6866314d873356369016cc6))
* **verify:** check s3tablescatalog and prove it survives teardown (T080, T081) ([8f38fcf](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/8f38fcff9b832fa77d6f29014646346e799c7d03))


### Bug Fixes

* address PR [#9](https://github.com/kbrockhoff/terraform-aws-lakehouse/issues/9) review findings ([492e7c1](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/492e7c11ce80b8ac6ae714d7de98b0d837777bba))
* **tenant:** manage row-filter exclusion labels with the singular LF-tag resource ([c7b0f1f](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/c7b0f1f54fac17637f934d67bdaa0971098af12c))
* **verify:** stop fixture owner and consumer link grants churning on re-apply ([0f0d686](https://github.com/kbrockhoff/terraform-aws-lakehouse/commit/0f0d68661023db3bd929c23f3532b97a3a583c5f))

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
