# coding=utf-8
from flask import Flask,request
import json

app = Flask(__name__)
app.debug=False

#100M*10
pages=1000000*10

table={str(i):i for i in range(pages)}

@app.route('/')
def index():
    key=request.args.get("key")
    result={
        "value":table[key]
    }
    return json.dumps(result)

if __name__ == "__main__":
    app.run(host="0.0.0.0",port=9000)