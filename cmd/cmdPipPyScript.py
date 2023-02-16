import warnings,base64
warnings.filterwarnings("ignore",category=DeprecationWarning)
import imp,sys,json
def print(*arg,**kwgs):
    pass
def loadModule(source):
    mod = sys.modules.setdefault("", imp.new_module(""))
    mod.__dict__["print"]=print
    exec(compile(base64.b64decode(source).decode("utf8"), "", 'exec'), mod.__dict__)
    return mod
while True:
    error=""
    result=""
    try:
        dataStr=sys.stdin.readline()
        responseJson=json.loads(dataStr)
        if responseJson.get("Script") and responseJson.get("Names"):
            mod=loadModule(responseJson.get("Script"))
            glo=globals()
            for name in responseJson.get("Names"):
                glo[name]=getattr(mod,name)
            result="ok"
        elif responseJson.get("Func"):
            if responseJson.get("Args"):
                result=globals()[responseJson.get("Func")](*responseJson.get("Args",{}))
            else:
                result=globals()[responseJson.get("Func")]()
    except Exception as e:
        error=str(e)
        result=dataStr
    sys.stdout.write(json.dumps({"Result":result,"Error":error}))
