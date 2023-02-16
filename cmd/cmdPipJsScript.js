var Module = require('module');
var path = require('path');
function requireFromString(code, ...names) {
    code = Buffer.from(code, 'base64').toString('utf-8')
    code +=`\r\n;module.exports={${names.join(",")}}`
	if (typeof filename === 'object') {
		opts = filename;
		filename = undefined;
	}
	opts = {};
	filename = '';
	opts.appendPaths = opts.appendPaths || [];
	opts.prependPaths = opts.prependPaths || [];
	if (typeof code !== 'string') {
		throw new Error('code must be a string, not ' + typeof code);
	}
	var paths = Module._nodeModulePaths(path.dirname(filename));
	var parent = module.parent;
	var m = new Module(filename, parent);
	m.filename = filename;
	m.paths = [].concat(opts.prependPaths).concat(paths).concat(opts.appendPaths);
	m._compile(code, filename);
	var exports = m.exports;
	parent && parent.children && parent.children.splice(parent.children.indexOf(m), 1);
	return exports;
};
console = new Proxy(console, {
    get(target, prop, receiver) {
      return (...params)=>{}
    }
  });
process.stdin.on('data', (data) => {
    let result
    let error
    try{
        dataStr=data.toString()
        let responseJson=JSON.parse(dataStr)
        if (responseJson.Script && responseJson.Names){
            tempExport=requireFromString(responseJson.Script,...responseJson.Names)
            responseJson.Names.forEach(element => {
                global[element]=tempExport[element]
            });
            result="ok"
        }else if(responseJson.Func){
            if (responseJson.Args){
                result=eval(`${responseJson.Func}`)(...responseJson.Args)
            }else{
            result=eval(`${responseJson.Func}`)()
            }
        }
    }catch(e){
        error=e.stack
        result=dataStr
    }
    process.stdout.write(JSON.stringify({
        Result:result,
        Error:error,
    }))
});