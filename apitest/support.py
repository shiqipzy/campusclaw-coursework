import json, urllib.request, urllib.error, subprocess, shlex, re, time
from pathlib import Path
class Context:
    def __init__(self,base,project):
        self.root=Path(__file__).resolve().parents[1];self.base=base;self.project=project
        assert re.fullmatch('[a-z0-9_-]+',project)
        self.env=dict(line.split('=',1) for line in (self.root/'.env').read_text(encoding='utf-8-sig').splitlines() if '=' in line and not line.startswith('#'))
        self.tokens={};self.state={};self.evidence={};self.secrets=[v for k,v in self.env.items() if ('PASSWORD' in k or 'SECRET' in k or 'TOKEN' in k or 'KEY' in k) and v]
        self.tag=str(int(time.time()))[-7:]
    def http(self,method,path,body=None,who='teacher_a',expected=200,form=None):
        headers={}
        if who in self.tokens:headers['Authorization']='Bearer '+self.tokens[who]
        data=None
        if body is not None:data=json.dumps(body,ensure_ascii=False).encode();headers['Content-Type']='application/json'
        if form:
            filename,content=form;boundary='campusclaw-test-boundary';data=(f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="{filename}"\r\nContent-Type: text/plain\r\n\r\n'.encode()+content.encode()+f'\r\n--{boundary}--\r\n'.encode());headers['Content-Type']='multipart/form-data; boundary='+boundary
        request=urllib.request.Request(self.base+path,data=data,headers=headers,method=method)
        try:response=urllib.request.urlopen(request,timeout=180)
        except urllib.error.HTTPError as exc:response=exc
        raw=response.read().decode('utf-8');assert response.status==expected,(method,path,response.status,raw[:500])
        if response.headers.get('Content-Type','').startswith('text/event-stream'):
            events=[]
            for block in raw.replace('\r\n','\n').split('\n\n'):
                if not block.strip():continue
                lines=block.splitlines();event=next(x[6:].strip() for x in lines if x.startswith('event:'));data=json.loads('\n'.join(x[5:].strip() for x in lines if x.startswith('data:')));events.append({'event':event,'data':data})
            return events
        return json.loads(raw) if raw and response.headers.get('Content-Type','').startswith('application/json') else raw
    def shell(self,command,stdin=None):
        if __import__('os').name=='nt':
            p=str(self.root).replace('\\','/');p='/mnt/'+p[0].lower()+p[2:]
            args=['wsl','-u','root','--','bash','-s']
        else:p=str(self.root);args=['sh','-s']
        script=f'cd {shlex.quote(p)}\ndocker compose -p {self.project} '+command
        if stdin is not None:script+=" <<'CAMPUS_SQL'\n"+stdin+"\nCAMPUS_SQL\n"
        else:script+='\n'
        result=subprocess.run(args,input=script.encode('utf-8'),capture_output=True,timeout=30)
        assert result.returncode==0,result.stderr.decode('utf-8',errors='replace')[-500:]
        return result.stdout.decode('utf-8',errors='replace').strip()
    def sql(self,query):return self.shell('exec -T db sh -c \'MYSQL_PWD="$MYSQL_PASSWORD" mysql --default-character-set=utf8mb4 -u"$MYSQL_USER" "$MYSQL_DATABASE" -N -B\'',query)
    def rpc(self,method,params):
        payload=json.dumps({'jsonrpc':'2.0','id':1,'method':method,'params':params},ensure_ascii=False)
        return json.loads(self.shell('exec -T mcp-demo sh -c '+shlex.quote('wget -q -O - --header="Content-Type: application/json" --header="Authorization: Bearer $MCP_AUTH_TOKEN" --post-data='+shlex.quote(payload)+' http://127.0.0.1:8090/mcp')))
    def chat(self,question,conv=None,who='student_a1'):
        conv=conv or self.state['conversation'];events=self.http('POST',f'/api/conversations/{conv}/messages',{'question':question,'class_id':2,'agent_id':999999},who=who)
        assert events[0]['event']=='meta';assert any(e['event']=='done' for e in events),events
        assert not any(e['event']=='error' for e in events),events
        text=''.join(e['data']['text'] for e in events if e['event']=='delta');assert len(text)>5
        tool_indexes=[i for i,e in enumerate(events) if e['event']=='tool_calls'];delta_indexes=[i for i,e in enumerate(events) if e['event']=='delta'];assert not tool_indexes or max(tool_indexes)<min(delta_indexes)
        detail=self.http('GET',f'/api/conversations/{conv}',who=who);assert detail['messages'][-1]['content']==text
        mid=detail['messages'][-1]['id'];assert self.sql(f'SELECT CHAR_LENGTH(content)>5 FROM messages WHERE id={mid};')=='1'
        return {'events':events,'answer':text,'message_id':mid,'conversation_id':conv}
