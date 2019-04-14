#!/usr/bin/env python3

from funcs import func_chain, map
from handlers import OfxDownload, OfxFiles
from itertools import chain
from ledger import Ledger, LedgerPosting, LedgerTransaction
from ofxclient.config import OfxConfig
from os import getenv
from pathlib import Path
from rules import RulesFile

import argparse
import sys


def apply_rules(rules: RulesFile, statement_transactions):
    for transactions in statement_transactions:
        yield map(rules.transform, transactions)


if __name__ == '__main__':
    parser = argparse.ArgumentParser()
    parser.add_argument('-c', '--config',
                        default=Path('~/ofxclient.ini').expanduser(),
                        help="Path to OFX Client ini file. Defaults to "
                        "~/ofxclient.ini")
    parser.add_argument('-r', '--rules',
                        default=getenv('LEDGER_RULES_FILE', default=''),
                        help="Path to an hledger CSV rules file. Defaults "
                        "to $LEDGER_RULES_FILE")
    parser.add_argument('-d', '--days', default=3, type=int,
                        help="Number of days to download from OFX"
                        "-connected institutions. Defaults to 3.")
    parser.add_argument('--open', '--opening-balances', action='store_true',
                        help="Add an 'opening balance' transaction, "
                        "calculated from the given OFX files or downloads.")
    parser.add_argument('--ledger', default=getenv('LEDGER_FILE', default=''),
                        help="Path to a ledger file. Defaults to $LEDGER_FILE")
    parser.add_argument('--sort', action='store_true',
                        help="Sort transactions by date. Default is not "
                        "guaranteed to be sorted.")
    parser.add_argument('ofx_file', nargs='*')
    args = parser.parse_args()

    if args.rules == "":
        parser.error("the following arguments are required: -r/--rules")
    rules = RulesFile.from_file(args.rules)
    c = OfxConfig(file_name=args.config)
    ledger = None
    if args.ledger != "":
        file_path = Path(args.ledger).expanduser()
        if file_path.exists():
            ledger = Ledger.from_file(file_path)

    if len(args.ofx_file) == 0:
        handler = OfxDownload(days=args.days, config=c)
    else:
        handler = OfxFiles(file_names=args.ofx_file, config=c)

    statement_transactions = apply_rules(rules, handler.transactions())

    if args.open is False:
        all_transactions = chain.from_iterable(statement_transactions)
    else:
        open_id = 'Opening-Balance'
        if ledger is not None and open_id in ledger:
            print('Error: Requested opening balance, but ledger already '
                  'contains an opening balance entry.', file=sys.stderr)
            sys.exit(2)
        all_transactions = []
        first_acct_txns = []

        for txns in statement_transactions:
            try:
                first_txn = next(txns)
                first_acct_txns.append(first_txn)
                all_transactions = chain(all_transactions, [first_txn], txns)
            except StopIteration:
                pass

        if len(first_acct_txns) == 0:
            print('Error: Could not find any transactions.', file=sys.stderr)
            sys.exit(1)

        opening_postings = list(func_chain(
            first_acct_txns,
            map(lambda t: t.postings[0]),
            map(lambda p: LedgerPosting(
                account=p.account,
                amount=p.balance - p.amount,
            )),
        ))
        opening_postings.append(LedgerPosting(
            id=open_id,
            account='equity:Opening Balances',
            amount=None,
            comment='id:' + open_id,
        ))
        opening_balance = LedgerTransaction(
            postings=opening_postings,
            date=min(map(lambda t: t.date, first_acct_txns)),
            description='* Opening Balance',
        )
        print(opening_balance)
    if args.sort:
        all_transactions = list(all_transactions)
        all_transactions.sort()
    if ledger is not None:
        for t in all_transactions:
            if t.postings[0].id not in ledger:
                print(t)
    else:
        for t in all_transactions:
            print(t)
