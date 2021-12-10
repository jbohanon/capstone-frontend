$(document).ready(() => {
    const searchBtn = document.getElementById('wikibook-search-btn')
    const searchText = document.getElementById('wikibook-search-text')
    searchBtn.addEventListener('click', function(e) {
        const inp = searchText.value
        fetchWrapper(`${addrRoot}/capstone/findtoken?search=${inp}`, 'GET', {}).then((resp) => {
            if (resp.status === 200) {
                createResultsList(document.getElementById('result-span'), resp.body, 50)
            } else {
                alert(resp.body)
            }
        })
    })

    Promise.all([
        fetchWrapper(`${addrRoot}/capstone/charts/uniquewords`, 'GET', {}).then((resp) => {
            setInnerHTML(document.getElementById('word-count-chart'), resp.body)
        }),
        fetchWrapper(`${addrRoot}/capstone/charts/pagecounts`, 'GET', {}).then((resp) => {
            setInnerHTML(document.getElementById('chapter-count-chart'), resp.body)
        }),
        fetchWrapper(`${addrRoot}/capstone/charts/externallinks`, 'GET', {}).then((resp) => {
            setInnerHTML(document.getElementById('link-count-chart'), resp.body)
        })
    ]).then()
})

const setInnerHTML = function(elem, html) {
    elem.innerHTML = html;
    Array.from(elem.querySelectorAll("script")).forEach( oldScript => {
        const newScript = document.createElement("script");
        Array.from(oldScript.attributes)
            .forEach( attr => newScript.setAttribute(attr.name, attr.value) );
        newScript.appendChild(document.createTextNode(oldScript.innerHTML));
        oldScript.parentNode.replaceChild(newScript, oldScript);
    });
}

function createResultsList(elem, arr, maxNumEl) {
    const resultArr = JSON.parse(arr)
    resultArr.forEach(function(v, i, a) {
        if(i <= maxNumEl) {
            v.book = v.book[0]

            const resSpan = document.createElement('span')
            const ln = document.createElement('a')
            const infoSpan = document.createElement('span')
            const similarBtn = document.createElement('button')
            ln.href = v.book.url
            ln.title = v.book.url
            ln.innerText = `${v.book.title.replace('Wikibooks: ', '')}`

            infoSpan.innerText = `Token: ${v.token}, Qty Occurrences: ${v.qty}`

            similarBtn.classList.add('button')
            similarBtn.innerText = 'Find Similar'
            similarBtn.id = v.docId

            similarBtn.addEventListener('click', function() {
                const lookupId = this.id
                fetchWrapper(`${addrRoot}/capstone/findsimilar?id=${lookupId}`, 'GET', {}).then((resp) => alert(resp))
            })

            resSpan.append(ln, infoSpan, similarBtn)
            elem.appendChild(resSpan)
        }
    })
}

function fetchWrapper(url, method, body) {
    return new Promise((resolve, reject) => {
            if (method === 'GET' || method === 'HEAD') {
                fetch(url).then((response) => returnTheValue(response, resolve))
            } else {
                fetch(url, {
                    method: method,
                    body: JSON.stringify(body)
                }).then((response) => returnTheValue(response, resolve))
            }
        }
    )
}

function returnTheValue(response, resolve) {
    response.text().then((responseBody) => resolve({ status: response.status, body: responseBody }))
}
