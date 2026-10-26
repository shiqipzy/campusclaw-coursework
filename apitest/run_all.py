"""Run URL-organized API cases against a dedicated Compose acceptance stack."""
import argparse, importlib.util, json, time, traceback, html
from pathlib import Path
from support import Context

def main():
    parser=argparse.ArgumentParser()
    parser.add_argument('--base-url',default='http://127.0.0.1:18081')
    parser.add_argument('--project',default='campusclaw_acceptance')
    args=parser.parse_args();ctx=Context(args.base_url,args.project)
    files=[]
    for path in (Path(__file__).parent/'cases').rglob('*.py'):
        spec=importlib.util.spec_from_file_location('case_'+str(len(files)),path)
        module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
        files.append((module.ORDER,path,module))
    results=[]
    for _,path,module in sorted(files):
        start=time.monotonic()
        try: detail=module.run(ctx);status='PASS'
        except Exception as exc: detail=traceback.format_exc();status='FAIL'
        results.append({'case':str(path.relative_to(Path(__file__).parent)),'status':status,'seconds':round(time.monotonic()-start,2),'detail':detail})
        print(status,results[-1]['case'],str(detail)[:180],flush=True)
        if status=='FAIL':break
    out=ctx.root/'deliverables'/'lesson56';out.mkdir(parents=True,exist_ok=True)
    report={'base_url':args.base_url,'project':args.project,'timestamp':time.strftime('%Y-%m-%d %H:%M:%S'),'cases':results,'evidence':ctx.evidence}
    raw=json.dumps(report,ensure_ascii=False,indent=2)
    for key in ctx.secrets:
        if key and key in raw:raise RuntimeError('Credential found in report')
    (out/'api-results.json').write_text(raw,encoding='utf-8')
    rows=''.join(f'<tr><td>{html.escape(r["case"])}</td><td>{r["status"]}</td><td>{r["seconds"]}</td><td><pre>{html.escape(str(r["detail"]))}</pre></td></tr>' for r in results)
    (out/'api-report.html').write_text('<!doctype html><meta charset="utf-8"><title>CampusClaw 接口验证</title><style>body{font:14px system-ui;margin:40px;color:#234}td,th{padding:12px;border-bottom:1px solid #ddd;text-align:left}pre{white-space:pre-wrap;max-width:700px}table{border-collapse:collapse}</style><h1>CampusClaw 第5/6课接口验证</h1><p>自动化验证；不代替真人同伴复核。HTML 报告按课程要求不随作业提交。</p><table><tr><th>接口脚本</th><th>结果</th><th>秒</th><th>核对内容</th></tr>'+rows+'</table>',encoding='utf-8')
    (out/'state.json').write_text(json.dumps(ctx.state,ensure_ascii=False,indent=2),encoding='utf-8')
    print(f'{sum(r["status"]=="PASS" for r in results)}/{len(files)} passed; evidence: {out}',flush=True)
    return 1 if len(results)!=len(files) or any(r['status']=='FAIL' for r in results) else 0
if __name__=='__main__':raise SystemExit(main())
