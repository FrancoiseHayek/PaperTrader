import os
from dotenv import load_dotenv
from alpaca_trade_api.rest import REST, TimeFrame

# Load env variables
load_dotenv()
API_KEY     = os.getenv("ALPACA_API_KEY")
API_SECRET  = os.getenv("ALPACA_API_SECRET")
BASE_URL    = os.getenv("ALPACA_BASE_URL")

# Instantiate the client
api = REST(API_KEY, API_SECRET, BASE_URL, api_version='v2')

# Test connectivity and account
account = api.get_account()
print(f"Account status: {account.status}, Buying Power: {account.buying_power}")