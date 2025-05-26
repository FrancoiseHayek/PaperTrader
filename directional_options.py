import os
from dotenv import load_dotenv
import pandas as pd
import numpy as np
from scipy.stats import norm
from datetime import datetime, time, timedelta
from alpaca_trade_api.rest import REST, TimeFrame


API_KEY    = os.getenv("ALPACA_API_KEY")
API_SECRET = os.getenv("ALPACA_API_SECRET")
BASE_URL   = os.getenv("ALPACA_BASE_URL")
START_DATE = "2024-01-01"
END_DATE   = "2025-01-01"

def fetch_and_save():
    if os.path.isfile(BARS_CSV):
        return
    api = REST(API_KEY, API_SECRET, BASE_URL)
    bars = api.get_bars(
        UNDERLYING_SYMBOL, TimeFrame.Minute,
        f"{START_DATE}T09:30:00Z",
        f"{END_DATE}T16:00:00Z",
        limit=None
    ).df
    bars = bars.between_time("09:30","16:00")
    bars.to_csv(BARS_CSV)

# --- CONFIGURATION & DATA LOADING ---
load_dotenv()
UNDERLYING_SYMBOL = "SPY"
BARS_CSV = f"{UNDERLYING_SYMBOL}_1min_bars.csv"
fetch_and_save()
bars = pd.read_csv(BARS_CSV, index_col=0, parse_dates=True)
bars.index = bars.index.tz_localize(None)
bars = bars.between_time("09:30","16:00")

# Starting capital
STARTING_BALANCE = 2000.0

# --- BLACK-SCHOLES PRICER ---
def bs_price(S, K, T, r, sigma, option_type="call"):
    """Return Black-Scholes price per share for a European option."""
    d1 = (np.log(S / K) + (r + 0.5 * sigma**2) * T) / (sigma * np.sqrt(T))
    d2 = d1 - sigma * np.sqrt(T)
    if option_type == "call":
        return S * norm.cdf(d1) - K * np.exp(-r * T) * norm.cdf(d2)
    else:
        return K * np.exp(-r * T) * norm.cdf(-d2) - S * norm.cdf(-d1)

def backtest_directional_scalp(
    bars: pd.DataFrame,
    starting_balance: float,
    entry_thresh=0.0015,
    ma_fast=5,
    ma_slow=10,
    tp_pct=0.15,
    sl_pct=0.15,
    r=0.0,
    sigma=0.25,
    max_contracts=10,
    vol_window=10,           # how many bars to average for volume
    vol_multiplier=1.2       # require current_vol >= multiplier * avg_vol
):
    balance   = starting_balance
    trades    = []
    prices    = []
    volumes   = []
    position  = None
    contracts = 0

    for ts, row in bars.iterrows():
        price  = row["close"]
        vol    = row["volume"]

        # rolling price & volume buffers
        prices.append(price)
        volumes.append(vol)
        if len(prices)  > ma_slow:   prices.pop(0)
        if len(volumes) > vol_window: volumes.pop(0)

        # only trade between 9:45 and 15:45, with enough data
        if not (time(9,45) <= ts.time() <= time(15,45)):
            continue
        if len(prices)  < ma_slow or len(volumes) < vol_window:
            continue

        ma_f = np.mean(prices[-ma_fast:])
        ma_s = np.mean(prices)
        avg_vol = np.mean(volumes)

        # ENTRY
        if position is None:
            # volume confirmation: skip if current bar is below avg
            if vol < vol_multiplier * avg_vol:
                continue

            # price‑threshold + MA crossover
            if ma_f > ma_s and (price/prices[-2] - 1) >= entry_thresh:
                side = "call"
            elif ma_f < ma_s and (prices[-2]/price - 1) >= entry_thresh:
                side = "put"
            else:
                continue

            # time to market close
            t_eod = datetime.combine(ts.date(), time(16,0))
            T = max((t_eod - ts).total_seconds(), 0) / (365*24*3600)

            # price the ATM option
            K       = price
            premium = bs_price(price, K, T, r, sigma, option_type=side)
            if not np.isfinite(premium) or premium <= 0:
                continue

            cost_per_contract = premium * 100
            if cost_per_contract <= 0:
                continue

            # size position
            contracts = min(int(balance // cost_per_contract), max_contracts)
            if contracts < 1:
                continue

            entry_premium = premium
            tp = entry_premium * (1 + tp_pct)
            sl = entry_premium * (1 - sl_pct)

            position = {
                "side":         side,
                "K":            K,
                "entry_ts":     ts,
                "entry_premium":entry_premium,
                "tp":           tp,
                "sl":           sl,
            }

        # EXIT
        else:
            t_eod = datetime.combine(ts.date(), time(16,0))
            T     = max((t_eod - ts).total_seconds(), 0) / (365*24*3600)

            premium = bs_price(price, position["K"], T, r, sigma, option_type=position["side"])
            # exit on TP, SL, or EOD
            if premium >= position["tp"] or premium <= position["sl"] or ts.time() >= time(15,59):
                exit_premium = premium
                pnl_per_share = exit_premium - position["entry_premium"]
                if position["side"] == "put":
                    pnl_per_share *= -1

                capital_used = contracts * position["entry_premium"] * 100
                realized_pnl = pnl_per_share * contracts * 100
                pct_change   = (realized_pnl / capital_used) * 100

                balance += realized_pnl

                trades.append({
                    "entry_ts":                 position["entry_ts"],
                    "exit_ts":                  ts,
                    "side":                     position["side"],
                    "contracts":                contracts,
                    "capital_used":             capital_used,
                    "entry_premium":            position["entry_premium"],
                    "exit_premium":             exit_premium,
                    "realized_pnl":             realized_pnl,
                    "pct_capital_used_change":  pct_change,
                    "balance_after":            balance
                })

                position  = None
                contracts = 0

    # wrap up
    df = pd.DataFrame(trades)
    if "realized_pnl" not in df.columns:
        df["realized_pnl"] = np.nan
    return df, balance


if __name__ == "__main__":
    df_trades, final_balance = backtest_directional_scalp(bars, STARTING_BALANCE)
    print(df_trades[['entry_ts','exit_ts','side','contracts', 'capital_used','realized_pnl', 'pct_capital_used_change','balance_after']].to_markdown())
    print(f"\nStarting balance: ${STARTING_BALANCE:,.2f}")
    print(f"Final balance:    ${final_balance:,.2f}")
    print(f"Net P&L:          ${final_balance - STARTING_BALANCE:,.2f}")
    print(f"ROI:              { (final_balance/STARTING_BALANCE -1)*100:.2f}%")
