# Sage [![Build Status](https://travis-ci.com/JohnStarich/sage.svg?branch=master)](https://travis-ci.com/JohnStarich/sage) [![Coverage Status](https://coveralls.io/repos/github/JohnStarich/sage/badge.svg?branch=master)](https://coveralls.io/github/JohnStarich/sage?branch=master)

Be your own accountant, without the stress.

Automatically download transactions from your banks and credit cards, then categorize them based on your own rules.

## Features

* [x] Automatically sync your ledger with banks and credit card institutions
* [x] Uses [double-entry bookkeeping][] to keep things in check
* [x] Can deploy as a single binary or as a Docker container

For future features, [see below](#future-work)

[double-entry bookkeeping]: https://en.wikipedia.org/wiki/Double-entry_bookkeeping_system

## Install

Choose one of the following options:

* Download and install the latest release from the [releases page](https://github.com/JohnStarich/sage/releases/latest) or this script:
```bash
curl -fsSL -H 'Accept: application/vnd.github.v3+json' https://api.github.com/repos/JohnStarich/sage/releases/latest | grep browser_download_url | cut -d '"' -f 4 | grep -i "$(uname -s)-$(uname -m)" | xargs curl -fSL -o sage
chmod +x sage
./sage -help  # Optionally move sage into your PATH
```
* Download the source and build it: `go get github.com/johnstarich/sage`
* Or pull the container image from [Docker Hub](https://hub.docker.com/r/johnstarich/sage): `docker pull johnstarich/sage`

Note: If you use the Docker image, the default command will look for the ledger and other setup files in `/data`. Example run command:
```bash
# ./data should contain ofxclient.ini, ledger.rules, and ledger.journal
docker run -d \
    --name sage \
    --volume "$PWD/data":/data \
    johnstarich/sage
```

## Usage

For available options, run `sage -help`

## Setup

Sage requires a ledger ([plain text accounting][]) file, an `ofxclient.ini` [credentials][ofxclient] file, and an [`hledger` rules][hledger rules] file.

[plain text accounting]: https://plaintextaccounting.org
[ofxclient]: https://github.com/captin411/ofxclient/#bank-information-storage
[hledger rules]: https://hledger.org/csv.html#csv-rules

The ledger will store all of your transactions in plain text so you can easily read it with any text editor. It also supports [several other tools][ledger tools] that can generate reports based on your ledger.

The `ofxclient.ini` file is currently generated by the [ofxclient][] CLI, but only supports passwords in the clear right now. (Plans for encrypted password stores coming in the future.)

The rules file is a format designed by the [hledger][] project for importing CSVs. This file will help Sage automatically categorize incoming transactions into the appropriate accounts for your ledger. After a transaction has been imported, it is assigned an account (category) from this file. To follow convention, only include rules to change the `account2` field or a `comment`. While changing `account1` is supported, it will likely cause problems with Sage since account1 is assumed to be the source institution of the transaction.

[hledger]: https://github.com/simonmichael/hledger

## Future work

* [ ] Web UI to view transactions, accounts, and balances
* [ ] Budget tracking (maybe add over-budget notifications)
* [ ] Automatic version control to reduce risk of data loss
* [ ] Smarter categorization by training on current ledger
* [ ] Web UI to add credentials for new accounts 
