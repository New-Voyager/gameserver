pipeline {
    agent any
    stages {
        stage('env') {
            steps {
                sh 'printenv | sort'
            }
        }
        stage('Hello') {
            steps {
                sh 'echo Hello 5'
                sh 'pwd'
                sh 'ls -l'
            }
        }
    }
}

