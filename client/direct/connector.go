package direct

import (
	"encoding/json"
	"net/url"
	"time"

	"github.com/aclindsa/ofxgo"
	"github.com/johnstarich/sage/client/model"
	sErrors "github.com/johnstarich/sage/errors"
	"github.com/johnstarich/sage/ledger"
	"github.com/johnstarich/sage/redactor"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	ofxAuthFailed = 15500
)

var (
	// ErrAuthFailed is returned whenever a signon request fails with an authentication problem
	ErrAuthFailed = errors.New("Username or password is incorrect")
)

// Connector downloads statements directly from an institution's OFX/QFX API
type Connector interface {
	model.Institution

	URL() string
	Username() string
	Password() redactor.String
	SetPassword(redactor.String)
	Config() Config
}

// Requestor can annotate an ofxgo.Request to fetch statements
type Requestor interface {
	Statement(req *ofxgo.Request, start, end time.Time) error
}

type directConnect struct {
	model.BasicInstitution

	ConnectorURL      string
	ConnectorUsername string
	ConnectorPassword redactor.String `json:",omitempty"`
	ConnectorConfig   Config
}

// New creates an institution that can automatically download statements
func New(
	description,
	fid,
	org,
	url,
	username, password string,
	config Config,
) Connector {
	return &directConnect{
		BasicInstitution: model.BasicInstitution{
			InstDescription: description,
			InstFID:         fid,
			InstOrg:         org,
		},
		ConnectorConfig:   config,
		ConnectorPassword: redactor.String(password),
		ConnectorURL:      url,
		ConnectorUsername: username,
	}
}

func (d *directConnect) URL() string {
	return d.ConnectorURL
}

func (d *directConnect) Username() string {
	return d.ConnectorUsername
}

func (d *directConnect) Password() redactor.String {
	return d.ConnectorPassword
}

func (d *directConnect) SetPassword(password redactor.String) {
	d.ConnectorPassword = password
}

func (d *directConnect) Config() Config {
	return d.ConnectorConfig
}

// UnmarshalConnector unmarshals the given bytes into a direct connector
func UnmarshalConnector(b []byte) (Connector, error) {
	var dc directConnect
	err := json.Unmarshal(b, &dc)
	return &dc, err
}

// ValidateConnector checks the state of the direct connector for correctness
func ValidateConnector(connector Connector) error {
	var errs sErrors.Errors
	if errs.ErrIf(connector == nil, "Direct connect must not be empty") {
		return errs.ErrOrNil()
	}
	errs.AddErr(model.ValidateInstitution(connector))
	errs.ErrIf(connector.URL() == "", "Institution URL must not be empty")
	u, err := url.Parse(connector.URL())
	if err != nil {
		errs.AddErr(errors.Wrap(err, "Institution URL is malformed"))
	} else {
		errs.ErrIf(u.Scheme != "https" && u.Hostname() != "localhost", "Institution URL is required to use HTTPS")
	}

	errs.ErrIf(connector.Username() == "", "Institution username must not be empty")
	errs.ErrIf(connector.Password() == "" && !IsLocalhostTestURL(connector.URL()), "Institution password must not be empty")
	config := connector.Config()
	errs.ErrIf(config.AppID == "", "Institution app ID must not be empty")
	errs.ErrIf(config.AppVersion == "", "Institution app version must not be empty")
	if !errs.ErrIf(config.OFXVersion == "", "Institution OFX version must not be empty") {
		_, err := ofxgo.NewOfxVersion(config.OFXVersion)
		errs.AddErr(err)
	}
	return errs.ErrOrNil()
}

// Statement downloads and returns transactions from a direct connector for the given time period
func Statement(connector Connector, start, end time.Time, requestors []Requestor, parser model.TransactionParser) ([]ledger.Transaction, error) {
	client, err := newSimpleClient(connector.URL(), connector.Config())
	if err != nil {
		return nil, err
	}

	return fetchTransactions(
		connector,
		start, end,
		requestors,
		// TODO it seems the ledger balance is nearly always the current balance, rather than the statement close. Restore this when a true closing balance can be found
		//balanceTransactions,
		client.Request,
		parser,
	)
}

func fetchTransactions(
	connector Connector,
	start, end time.Time,
	requestors []Requestor,
	doRequest func(*ofxgo.Request) (*ofxgo.Response, error),
	parse model.TransactionParser,
) ([]ledger.Transaction, error) {
	var query ofxgo.Request
	for _, r := range requestors {
		if err := r.Statement(&query, start, end); err != nil {
			return nil, err
		}
	}
	if len(query.Bank) == 0 && len(query.CreditCard) == 0 {
		return nil, errors.Errorf("Invalid statement query: does not contain any statement requests: %+v", query)
	}

	addSignonRequest(connector, &query)

	response, err := doRequest(&query)
	if err != nil {
		return nil, err
	}

	if response.Signon.Status.Code != 0 {
		if response.Signon.Status.Code == ofxAuthFailed {
			return nil, ErrAuthFailed
		}
		meaning, err := response.Signon.Status.CodeMeaning()
		if err != nil {
			return nil, errors.Wrap(err, "Failed to parse OFX response code")
		}
		return nil, errors.Errorf("Nonzero signon status (%d: %s) with message: %s", response.Signon.Status.Code, meaning, response.Signon.Status.Message)
	}

	_, txns, err := parse(response)
	return txns, err
}

// Verify attempts to sign in with the given account. Returns any encountered errors
func Verify(connector Connector, requestor Requestor, parser model.TransactionParser) error {
	end := time.Now()
	start := end.Add(-24 * time.Hour)
	_, err := Statement(connector, start, end, []Requestor{requestor}, parser)
	return err
}

func addSignonRequest(connector Connector, req *ofxgo.Request) {
	config := connector.Config()
	req.URL = connector.URL()
	req.Signon = ofxgo.SignonRequest{
		ClientUID: ofxgo.UID(config.ClientID),
		Org:       ofxgo.String(connector.Org()),
		Fid:       ofxgo.String(connector.FID()),
		UserID:    ofxgo.String(connector.Username()),
		UserPass:  ofxgo.String(connector.Password()),
	}
}

// Accounts fetches available accounts at the direct connector's institution
func Accounts(connector Connector, logger *zap.Logger) ([]model.Account, error) {
	client, err := newSimpleClient(connector.URL(), connector.Config())
	if err != nil {
		return nil, err
	}
	return accounts(connector, logger, client.Request)
}

func accounts(connector Connector, logger *zap.Logger, doRequest func(*ofxgo.Request) (*ofxgo.Response, error)) ([]model.Account, error) {
	var query ofxgo.Request
	uid, err := ofxgo.RandomUID()
	if err != nil {
		return nil, err
	}
	query.Signup = append(query.Signup, &ofxgo.AcctInfoRequest{
		TrnUID: *uid,
	})
	addSignonRequest(connector, &query)

	resp, err := doRequest(&query)
	if err != nil {
		return nil, err
	}
	if len(resp.Signup) == 0 {
		return nil, errors.New("Response did not contain any messages")
	}

	acctInfoResp, ok := resp.Signup[0].(*ofxgo.AcctInfoResponse)
	if !ok {
		return nil, errors.Errorf("Unknown account info response type: %T", resp.Signup[0])
	}
	var accounts []model.Account
	for _, acctInfo := range acctInfoResp.AcctInfo {
		if account, ok := parseAcctInfo(connector, acctInfo, logger); ok {
			accounts = append(accounts, account)
		}
	}
	return accounts, nil
}

func parseAcctInfo(connector Connector, acctInfo ofxgo.AcctInfo, logger *zap.Logger) (model.Account, bool) {
	accountName := acctInfo.Desc.String()
	if accountName == "" {
		accountName = acctInfo.Name.String()
	}
	logger = logger.With(zap.String("name", accountName))
	switch {
	case acctInfo.BankAcctInfo != nil:
		bankID := acctInfo.BankAcctInfo.BankAcctFrom.BankID.String()
		accountID := acctInfo.BankAcctInfo.BankAcctFrom.AcctID.String()
		accountTypeStr := acctInfo.BankAcctInfo.BankAcctFrom.AcctType.String()
		accountType := ParseAccountType(accountTypeStr)
		// TODO add branch ID, acct key support for non-USA

		logger = logger.With(zap.String("accountID", accountID))
		if !acctInfo.BankAcctInfo.SupTxDl {
			logger.Warn("Bank account does not support downloading transactions")
			return nil, false
		}
		if accountName == "" {
			accountName = accountID
		}
		switch accountType {
		case CheckingType:
			return NewCheckingAccount(accountID, bankID, accountName, connector), true
		case SavingsType:
			return NewSavingsAccount(accountID, bankID, accountName, connector), true
		default:
			logger.Warn("Bank account is of unsupported type", zap.String("type", accountTypeStr))
			return nil, false
		}
	case acctInfo.CCAcctInfo != nil:
		accountID := acctInfo.CCAcctInfo.CCAcctFrom.AcctID.String()
		logger = logger.With(zap.String("accountID", accountID))
		if !acctInfo.CCAcctInfo.SupTxDl {
			logger.Warn("Credit card account does not support downloading transactions")
			return nil, false
		}
		if accountName == "" {
			accountName = accountID
		}
		return NewCreditCard(accountID, accountName, connector), true
	default:
		logger.Warn("Account was not a bank or credit card account")
		return nil, false
	}
}
