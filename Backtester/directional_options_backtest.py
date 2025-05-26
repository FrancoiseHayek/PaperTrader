import os
from dotenv import load_dotenv
import pandas as pd
import numpy as np
from itertools import product
from joblib import Parallel, delayed
import multiprocessing
from tqdm.auto import tqdm

from Algorithms.directional_options import backtest_directional_scalp  # your backtest fn

# --- LOAD BAR DATA ---
UNDERLYING_SYMBOL = "SPY"
BARS_CSV = f"{UNDERLYING_SYMBOL}_1min_bars.csv"
bars = pd.read_csv(BARS_CSV, index_col=0, parse_dates=True).between_time("09:30","16:00")
if hasattr(bars.index, 'tz'):
    bars.index = bars.index.tz_localize(None)

# --- PARAMETER GRID ---
param_grid = {
    "entry_thresh":   [0.001, 0.0015, 0.002],
    "ma_fast":        [3, 5, 7],
    "ma_slow":        [10, 15, 20],
    "tp_pct":         [0.1, 0.15, 0.2],         
    "sl_pct":         [0.1, 0.15, 0.2],     
    "sigma":          [0.2, 0.25, 0.3],
    "max_contracts":  [10]
}

# --- WORKER FUNCTION (updated) ---
def evaluate_params(vals):
    # build param dict
    params = dict(zip(param_grid.keys(), vals))

    # run backtest_directional_scalp → now returns (df_trades, final_balance)
    df_trades, final_balance = backtest_directional_scalp(
        bars.copy(),
        starting_balance=2000,          # <-- pass your starting balance
        entry_thresh=params["entry_thresh"],
        ma_fast=params["ma_fast"],
        ma_slow=params["ma_slow"],
        tp_pct=params["tp_pct"],
        sl_pct=params["sl_pct"],
        sigma=params["sigma"],
        max_contracts=params["max_contracts"]
    )

    # compute traditional metrics
    if df_trades.empty:
        trade_metrics = {
            "trades":    0,
            "win_rate":  np.nan,
            "avg_win":   np.nan,
            "avg_loss":  np.nan,
            "expectancy":np.nan,
        }
    else:
        df_trades["win"] = df_trades["pnl"] > 0
        trade_metrics = {
            "trades":    len(df_trades),
            "win_rate":  df_trades["win"].mean(),
            "avg_win":   df_trades.loc[df_trades["win"], "pnl"].mean(),
            "avg_loss":  df_trades.loc[~df_trades["win"], "pnl"].mean(),
            "expectancy":df_trades["pnl"].mean(),
        }

    # compute balance‐based metrics
    net_pnl = final_balance - 2000
    roi     = net_pnl / 2000

    balance_metrics = {
        "final_balance": final_balance,
        "net_pnl":       net_pnl,
        "roi":           roi,
    }

    # merge params + trade metrics + balance metrics
    return {**params, **trade_metrics, **balance_metrics}



# --- PARALLEL SWEEP ---
all_param_values = list(product(*param_grid.values()))
num_cores = multiprocessing.cpu_count()

# Use tqdm to track progress
results = Parallel(n_jobs=num_cores)(
    delayed(evaluate_params)(vals) for vals in tqdm(all_param_values)
)

# --- AGGREGATE RESULTS ---
sweep_df = pd.DataFrame(results)
best = sweep_df.sort_values("expectancy", ascending=False).head(10)

print("Top 10 parameter sets by expectancy:")
print(best.to_string(index=False))

# Optionally save full sweep
sweep_df.to_csv("directional_scalp_sweep_results_parallel.csv", index=False)


best_by_roi = sweep_df.sort_values("roi", ascending=False).head(10)
print("Top 10 by ROI:")
print(best_by_roi.to_string(index=False))
