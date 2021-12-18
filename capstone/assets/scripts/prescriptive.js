//import * as echarts from 'echarts';
//import * as ecStat from 'ecStat';

function renderPrescriptive(data) {
    let myChart = echarts.init(document.getElementById('prescriptive-chart'));
    let option;

    echarts.registerTransform(ecStat.transform.regression);
    option = {
        dataset: [
            {
                source: data
            },
            {
                transform: {
                    type: 'ecStat:regression'
                }
            }
        ],
        title: {
            text: 'External Links By Unique Token Count',
            left: 'center'
        },
        legend: {
            bottom: 5
        },
        tooltip: {
            trigger: 'axis',
            axisPointer: {
                type: 'cross'
            }
        },
        xAxis: {
            splitLine: {
                lineStyle: {
                    type: 'dashed'
                }
            }
        },
        yAxis: {
            splitLine: {
                lineStyle: {
                    type: 'dashed'
                }
            }
        },
        series: [
            {
                name: 'scatter',
                type: 'scatter'
            },
            {
                name: 'line',
                type: 'line',
                datasetIndex: 1,
                symbolSize: 0.1,
                symbol: 'circle',
                label: { show: true, fontSize: 16 },
                labelLayout: { dx: -20 },
                encode: { label: 2, tooltip: 1 }
            }
        ]
    };

    option && myChart.setOption(option);

}